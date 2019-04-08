// Demo is an interactive demonstration of the Go SDK using the Stellar TestNet.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/stellar/go/clients/horizon"
	horizonclient "github.com/stellar/go/exp/clients/horizon"
	"github.com/stellar/go/exp/txnbuild"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/errors"

	"github.com/stellar/go/keypair"
)

const friendbotAddress = "GAIH3ULLFQ4DGSECF2AR555KZ4KNDGEKN4AFI4SU2M7B43MGK3QJZNSR"

func main() {
	resetF := flag.Bool("reset", false, "Remove all testing state")
	flag.Parse()

	keys := initKeys()
	client := horizonclient.DefaultTestNetClient

	if *resetF {
		fmt.Println("Resetting TestNet state...")
		reset(client, keys)
		fmt.Println("Reset complete.")
	}
}

func reset(client *horizonclient.Client, keys []key) {
	for i, k := range keys {
		// Check if test0 account exists
		accountRequest := horizonclient.AccountRequest{AccountId: k.Address}
		horizonSourceAccount, err := client.AccountDetail(accountRequest)
		if err != nil {
			fmt.Printf("    Couldn't get account detail for %s: %s\n", k.Address, err)
			fmt.Printf("    Skipping further operations on account %s.\n", k.Address)
			continue
		}
		k.Account.FromHorizonAccount(horizonSourceAccount)
		keys[i].Account.FromHorizonAccount(horizonSourceAccount)

		// It exists - so we will proceed to delete it
		fmt.Println("\n    Found testnet account with ID:", k.Account.ID)

		// Find any offers that need deleting...
		offerRequest := horizonclient.OfferRequest{
			ForAccount: k.Address,
			Cursor:     "now",
			Order:      horizonclient.OrderDesc,
		}
		offers, err := client.Offers(offerRequest)
		dieIfError("error while getting offers", err)
		fmt.Printf("    Account %s has %v offers:\n", k.Address, len(offers.Embedded.Records))

		// ...and delete them
		for _, o := range offers.Embedded.Records {
			fmt.Println("    ", o)
			txe, err := deleteOffer(k.Account, uint64(o.ID), k)
			dieIfError("Problem building deleteOffer op", err)
			fmt.Printf("        Deleting offer %d...\n", o.ID)
			resp := submit(client, txe)
			fmt.Println(resp.TransactionSuccessToString())
			k.Account.SequenceNumber++
		}

		// Find any authorised trustlines on this account...
		fmt.Printf("    Account %s has %d balances...\n", k.Address, len(horizonSourceAccount.Balances))

		// ...and delete them
		for _, b := range horizonSourceAccount.Balances {
			// Native balances don't have trustlines
			if b.Type == "native" {
				continue
			}
			asset := txnbuild.Asset{}
			hAsset := horizon.Asset{
				Type:   b.Type,
				Code:   b.Code,
				Issuer: b.Issuer,
			}
			asset.FromHorizonAsset(hAsset)

			// Send the asset back to the issuer...
			fmt.Printf("        Sending %v of surplus asset %s:%s back to issuer...\n", b.Balance, hAsset.Code, hAsset.Issuer)
			txe, err := payment(k.Account, hAsset.Issuer, b.Balance, asset, k)
			dieIfError("Problem building payment op", err)
			resp := submit(client, txe)
			fmt.Println(resp.TransactionSuccessToString())
			k.Account.SequenceNumber++

			// Delete the now-empty trustline...
			fmt.Printf("        Deleting trustline for asset %s:%s...\n", b.Code, b.Issuer)
			txe, err = deleteTrustline(k.Account, asset, k)
			dieIfError("Problem building deleteTrustline op", err)
			resp = submit(client, txe)
			fmt.Println(resp.TransactionSuccessToString())
			k.Account.SequenceNumber++
		}

		// Find any data entries on this account...
		fmt.Printf("    Account %s has %d data entries...\n", k.Address, len(horizonSourceAccount.Data))
		for dataKey := range horizonSourceAccount.Data {
			decodedV, _ := horizonSourceAccount.GetData(dataKey)
			fmt.Printf("    Deleting data entry '%s' -> '%s'...\n", dataKey, decodedV)
			txe, err := deleteData(k.Account, dataKey, k)
			dieIfError("Problem building manageData op", err)
			resp := submit(client, txe)
			fmt.Println(resp.TransactionSuccessToString())
			k.Account.SequenceNumber++
		}
	}

	// Merge the accounts...
	for _, k := range keys {
		if k.Account == (txnbuild.Account{}) {
			continue
		}
		fmt.Printf("    Merging account %s back to friendbot (%s)...\n", k.Address, friendbotAddress)
		txe, err := mergeAccount(k.Account, friendbotAddress, k)
		dieIfError("Problem building mergeAccount op", err)
		resp := submit(client, txe)
		fmt.Println(resp.TransactionSuccessToString())
	}
}

func deleteData(source txnbuild.Account, k string, signer key) (string, error) {
	manageData := txnbuild.ManageData{
		Name: k,
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&manageData},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func payment(source txnbuild.Account, dest, amount string, asset txnbuild.Asset, signer key) (string, error) {
	payment := txnbuild.Payment{
		Destination: dest,
		Amount:      amount,
		Asset:       &asset,
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&payment},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func deleteTrustline(source txnbuild.Account, asset txnbuild.Asset, signer key) (string, error) {
	deleteTrustline := txnbuild.NewRemoveTrustlineOp(&asset)

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&deleteTrustline},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func deleteOffer(source txnbuild.Account, offerID uint64, signer key) (string, error) {
	deleteOffer := txnbuild.NewDeleteOfferOp(offerID)

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&deleteOffer},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func mergeAccount(source txnbuild.Account, destAddress string, signer key) (string, error) {
	accountMerge := txnbuild.AccountMerge{
		Destination: destAddress,
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&accountMerge},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

type key struct {
	Seed    string
	Address string
	Account txnbuild.Account
	Keypair *keypair.Full
}

func initKeys() []key {
	// Accounts created on testnet
	keys := []key{
		// test0
		key{Seed: "SBPQUZ6G4FZNWFHKUWC5BEYWF6R52E3SEP7R3GWYSM2XTKGF5LNTWW4R",
			Address: "GDQNY3PBOJOKYZSRMK2S7LHHGWZIUISD4QORETLMXEWXBI7KFZZMKTL3",
		},
		// test1
		key{Seed: "SBMSVD4KKELKGZXHBUQTIROWUAPQASDX7KEJITARP4VMZ6KLUHOGPTYW",
			Address: "GAS4V4O2B7DW5T7IQRPEEVCRXMDZESKISR7DVIGKZQYYV3OSQ5SH5LVP",
		},
		// test2
		key{Seed: "SBZVMB74Z76QZ3ZOY7UTDFYKMEGKW5XFJEB6PFKBF4UYSSWHG4EDH7PY",
			Address: "GB7BDSZU2Y27LYNLALKKALB52WS2IZWYBDGY6EQBLEED3TJOCVMZRH7H"},
		// dev-null
		key{Seed: "SD3ZKHOPXV6V2QPLCNNH7JWGKYWYKDFPFRNQSKSFF3Q5NJFPAB5VSO6D",
			Address: "GBAQPADEYSKYMYXTMASBUIS5JI3LMOAWSTM2CHGDBJ3QDDPNCSO3DVAA"},
	}

	for i, k := range keys {
		myKeypair, err := keypair.Parse(k.Seed)
		dieIfError("keypair didn't parse!", err)
		keys[i].Keypair = myKeypair.(*keypair.Full)
	}

	return keys
}

func submit(client *horizonclient.Client, txeBase64 string) (resp horizon.TransactionSuccess) {
	resp, err := client.SubmitTransaction(txeBase64)
	if err != nil {
		hError := err.(*horizonclient.Error)
		err = printHorizonError(hError)
		dieIfError("couldn't print Horizon eror", err)
		os.Exit(1)
	}

	return
}

func dieIfError(desc string, err error) {
	if err != nil {
		log.Fatalf("Fatal error (%s): %s", desc, err)
	}
}

func printHorizonError(hError *horizonclient.Error) error {
	problem := hError.Problem
	log.Println("Error type:", problem.Type)
	log.Println("Error title:", problem.Title)
	log.Println("Error status:", problem.Status)
	log.Println("Error detail:", problem.Detail)
	log.Println("Error instance:", problem.Instance)

	resultCodes, err := hError.ResultCodes()
	if err != nil {
		return errors.Wrap(err, "Couldn't read ResultCodes")
	}
	log.Println("TransactionCode:", resultCodes.TransactionCode)
	log.Println("OperationCodes:")
	for _, code := range resultCodes.OperationCodes {
		log.Println("    ", code)
	}

	resultString, err := hError.ResultString()
	if err != nil {
		return errors.Wrap(err, "Couldn't read ResultString")
	}
	log.Println("TransactionResult XDR (base 64):", resultString)

	envelope, err := hError.Envelope()
	if err != nil {
		return errors.Wrap(err, "Couldn't read Envelope")
	}
	log.Println("TransactionEnvelope XDR:", envelope)

	return nil
}
