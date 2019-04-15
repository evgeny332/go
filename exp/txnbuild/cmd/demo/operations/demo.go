// Package demo is an interactive demonstration of the Go SDK using the Stellar TestNet.
package demo

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/stellar/go/clients/horizon"
	horizonclient "github.com/stellar/go/exp/clients/horizon"
	"github.com/stellar/go/exp/txnbuild"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/errors"

	"github.com/stellar/go/keypair"
)

// TODO:
// 1) Randomly generate the test account addresses. Use a file to store them so they can be deleted/referred to.
// 2) Clean up printing output
// 3) Add missing operations

const friendbotAddress = "GAIH3ULLFQ4DGSECF2AR555KZ4KNDGEKN4AFI4SU2M7B43MGK3QJZNSR"

func loadAccounts(client *horizonclient.Client, keys []key) []key {
	for i, k := range keys {
		accountRequest := horizonclient.AccountRequest{AccountID: k.Address}
		horizonSourceAccount, err := client.AccountDetail(accountRequest)
		if err == nil {
			keys[i].Account = &horizonSourceAccount
			keys[i].Exists = true
		}
	}

	return keys
}

// Reset removes all test accounts created by this demo. All funds are transferred back to Friendbot.
func Reset(client *horizonclient.Client, keys []key) {
	keys = loadAccounts(client, keys)
	for _, k := range keys {
		if !k.Exists {
			fmt.Printf("    Account %s not found - skipping further operations on it...\n", k.Address)
			continue
		}

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
		}

		// Find any authorised trustlines on this account...
		fmt.Printf("    Account %s has %d balances...\n", k.Address, len(k.Account.Balances))

		// ...and delete them
		for _, b := range k.Account.Balances {
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

			// Delete the now-empty trustline...
			fmt.Printf("        Deleting trustline for asset %s:%s...\n", b.Code, b.Issuer)
			txe, err = deleteTrustline(k.Account, asset, k)
			dieIfError("Problem building deleteTrustline op", err)
			resp = submit(client, txe)
			fmt.Println(resp.TransactionSuccessToString())
		}

		// Find any data entries on this account...
		fmt.Printf("    Account %s has %d data entries...\n", k.Address, len(k.Account.Data))
		for dataKey := range k.Account.Data {
			decodedV, _ := k.Account.GetData(dataKey)
			fmt.Printf("    Deleting data entry '%s' -> '%s'...\n", dataKey, decodedV)
			txe, err := deleteData(k.Account, dataKey, k)
			dieIfError("Problem building manageData op", err)
			resp := submit(client, txe)
			fmt.Println(resp.TransactionSuccessToString())
		}
	}

	// Merge the accounts...
	for _, k := range keys {
		if !k.Exists {
			continue
		}
		fmt.Printf("    Merging account %s back to friendbot (%s)...\n", k.Address, friendbotAddress)
		txe, err := mergeAccount(k.Account, friendbotAddress, k)
		dieIfError("Problem building mergeAccount op", err)
		resp := submit(client, txe)
		fmt.Println(resp.TransactionSuccessToString())
	}
}

// Initialise funds an initial set of accounts for use with other demo operations. The first account is
// funded from Friendbot; subseqeuent accounts are created and funded from this first account.
func Initialise(client *horizonclient.Client, keys []key) {
	// Fund the first account from friendbot
	fmt.Printf("    Funding account %s from friendbot...\n", keys[0].Address)
	_, err := fund(keys[0].Address)
	dieIfError(fmt.Sprintf("Couldn't fund account %s from friendbot", keys[0].Address), err)

	keys = loadAccounts(client, keys)

	// Fund the others using the create account operation
	for i := 1; i < len(keys); i++ {
		fmt.Printf("    Funding account %s from account %s...\n", keys[i].Address, keys[0].Address)
		txe, err := createAccount(keys[0].Account, keys[i].Address, keys[0])
		dieIfError("Problem building createAccount op", err)
		resp := submit(client, txe)
		fmt.Println(resp.TransactionSuccessToString())
	}
}

func fund(address string) (resp *http.Response, err error) {
	resp, err = http.Get("https://friendbot.stellar.org/?addr=" + address)
	if err != nil {
		return nil, err
	}
	return
}

func createAccount(source *horizon.Account, dest string, signer key) (string, error) {
	createAccountOp := txnbuild.CreateAccount{
		Destination: dest,
		Amount:      "100",
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&createAccountOp},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func deleteData(source *horizon.Account, dataKey string, signer key) (string, error) {
	manageDataOp := txnbuild.ManageData{
		Name: dataKey,
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&manageDataOp},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func manageData(source *horizon.Account, dataKey string, dataValue string, signer key) (string, error) {
	manageDataOp := txnbuild.ManageData{
		Name:  dataKey,
		Value: []byte(dataValue),
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&manageDataOp},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func payment(source *horizon.Account, dest, amount string, asset txnbuild.Asset, signer key) (string, error) {
	paymentOp := txnbuild.Payment{
		Destination: dest,
		Amount:      amount,
		Asset:       &asset,
	}

	tx := txnbuild.Transaction{
		SourceAccount: source,
		Operations:    []txnbuild.Operation{&paymentOp},
		Network:       network.TestNetworkPassphrase,
	}

	txeBase64, err := tx.BuildSignEncode(signer.Keypair)
	if err != nil {
		return "", errors.Wrap(err, "couldn't serialise transaction")
	}

	return txeBase64, nil
}

func deleteTrustline(source *horizon.Account, asset txnbuild.Asset, signer key) (string, error) {
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

func deleteOffer(source *horizon.Account, offerID uint64, signer key) (string, error) {
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

func mergeAccount(source *horizon.Account, destAddress string, signer key) (string, error) {
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
	Account *horizon.Account
	Keypair *keypair.Full
	Exists  bool
}

func InitKeys() []key {
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
