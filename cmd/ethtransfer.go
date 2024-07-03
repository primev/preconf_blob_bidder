package main

import (
	"flag"
	"log"
	"math/big"

	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

func sendTransfer() {
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	flag.Parse()
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	// Start Client
	client, err := bb.NewGethClient(*endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to client: %v", err)
	}

	// Authenticate address
	authAcct, err := bb.AuthenticateAddress(*privateKeyHex, client)
	if err != nil {
		log.Fatalf("Failed to authenticate private key: %v", err)
	}

	// Send ETH Transfer
	txHash, err := ee.SelfETHTransfer(client, *authAcct, big.NewInt(100000), 3000000, []byte{0x4c, 0xdc, 0xeb, 0x20})
	if err != nil {
		log.Fatalf("Failed to send transaction: %v", err)
	}

	log.Printf("tx sent: %s", txHash)
}
