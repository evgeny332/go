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

func main() {
	resetF := flag.Bool("reset", false, "Remove all testing state")
	flag.Parse()

	keys := initKeys()
	client := horizonclient.DefaultTestNetClient

	if *resetF {
		fmt.Println("Resetting TestNet state...")
		reset(client, keys)
		fmt.Println("Reset complete")
	}
}

func reset(client *horizonclient.Client, keys []key) {
	// Check if test0 account exists
	accountRequest := horizonclient.AccountRequest{AccountId: keys[0].Address}
	horizonSourceAccount, err := client.AccountDetail(accountRequest)
	dieIfError(fmt.Sprintf("couldn't get account detail for %s", keys[0].Address), err)
	sourceAccount := txnbuild.Account{}
	sourceAccount.FromHorizonAccount(horizonSourceAccount)

	// It exists - so we will proceed to delete it
	fmt.Println("Found testnet account with ID:", sourceAccount.ID)

	// Find any offers that need deleting...
	offerRequest := horizonclient.OfferRequest{
		ForAccount: keys[0].Address,
		Cursor:     "now",
		Order:      horizonclient.OrderDesc,
	}
	offers, err := client.Offers(offerRequest)
	dieIfError("error while getting offers", err)
	fmt.Printf("Account %s has %v offers:\n", keys[0].Address, len(offers.Embedded.Records))

	// ...and delete them
	for _, o := range offers.Embedded.Records {
		fmt.Println("    ", o)
		txe, err := deleteOffer(sourceAccount, uint64(o.ID), keys[0])
		dieIfError("Problem building deleteOffer op", err)
		fmt.Printf("Deleting offer %d...\n", o.ID)
		resp := submit(client, txe)
		fmt.Println(resp.TransactionSuccessToString())
		sourceAccount.SequenceNumber++
	}

	// Find any issued assets for this account...
	assetRequest := horizonclient.AssetRequest{ForAssetIssuer: keys[0].Address}
	assets, err := client.Assets(assetRequest)
	dieIfError(fmt.Sprintf("error while getting issued assets for account %s", keys[0].Address), err)
	fmt.Printf("Account %s has %v issued assets:\n", keys[0].Address, len(assets.Embedded.Records))
	for _, a := range assets.Embedded.Records {
		fmt.Println("    ", a)

		asset := txnbuild.Asset{}
		hAsset := horizon.Asset{
			Type:   a.Type,
			Code:   a.Code,
			Issuer: a.Issuer,
		}
		asset.FromHorizonAsset(hAsset)

		txe, err := deleteTrustline(sourceAccount, asset, keys[0])
		dieIfError("Problem building deleteTrustline op", err)
		fmt.Printf("Deleting trustline %v...\n", asset)
		resp := submit(client, txe)
		fmt.Println(resp.TransactionSuccessToString())
		sourceAccount.SequenceNumber++
	}

	// Merge the account
	txe, err := mergeAccount(sourceAccount, keys[3], keys[0])
	dieIfError("Problem building mergeAccount op", err)

	resp := submit(client, txe)
	fmt.Println(resp.TransactionSuccessToString())
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
	log.Println("Base 64 TX: ", txeBase64)

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
	log.Println("Base 64 TX: ", txeBase64)

	return txeBase64, nil
}

func mergeAccount(source txnbuild.Account, dest key, signer key) (string, error) {
	accountMerge := txnbuild.AccountMerge{
		Destination: dest.Address,
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
	log.Println("Base 64 TX: ", txeBase64)

	return txeBase64, nil
}

type key struct {
	Seed    string
	Address string
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
