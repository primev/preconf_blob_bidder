package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

func sendETHTransfer() {

	cfg := bb.BidderConfig{
		// ServerAddress: "localhost:13524", // Default address for mevcommit gRPC server //
		ServerAddress: "127.0.0.1:13524",
		LogFmt:        "json", // Example log format
		LogLevel:      "info", // Example log level
	}

	bidderClient, err := bb.NewBidderClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v. Remember to connect to the mev-commit p2p bidder node.", err)
	}
	fmt.Println("Connected to mev-commit client")

	// TODO 7/10 min deposit no longer exists in 0.4.0 release
	// Get the minimum deposit and deposit the minimum amount in the current bid window
	// response, err := bidderClient.GetMinDeposit()
	// if err != nil {
	// 	log.Fatalf("Failed to get minimum deposit: %v", err)
	// }
	// fmt.Printf("Minimum deposit required: %v\n", response.Amount)

	// TODO 7/10 as of 0.4.0 release, depositMinBidAmount no longer seems to exist
	// windowNumber, err := bidderClient.DepositMinBidAmount()
	// if err != nil {
	// 	log.Fatalf("Failed to deposit minimum bid amount: %v", err)
	// }
	// fmt.Printf("Deposited into window: %v\n", windowNumber)

	// Start Holesky client with command line flags
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	flag.Parse()
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	client, err := bb.NewGethClient(*endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to client: %v", err)
	}

	// Authenticate address with private key
	authAcct, err := bb.AuthenticateAddress(*privateKeyHex, client)
	if err != nil {
		log.Fatalf("Failed to authenticate private key: %v", err)
	}

	// Get current block number
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to retrieve block number: %v", err)
	}
	fmt.Printf("Current block number: %v\n", blockNumber)

	// Send ETH Transfer
	txHash, err := ee.SelfETHTransfer(client, *authAcct, big.NewInt(100000), 3000000, []byte{0x4c, 0xdc, 0xeb, 0x20})
	if err != nil {
		log.Fatalf("Failed to send transaction: %v", err)
	}

	log.Printf("tx sent: %s", txHash)

	// Convert uint64 to int64. Add +1 to be the next block number
	blockNumberInt64 := int64(blockNumber) + 1
	// print the preconf block number
	fmt.Printf("Preconf block number: %v\n", blockNumberInt64)
	currentTime := time.Now().UnixMilli()
	// bid preconf parameters
	txHashes := []string{strings.TrimPrefix(txHash, "0x")}
	amount := "1000000000000" // Specify amount in wei
	decayStart := currentTime - (time.Duration(8 * time.Second).Milliseconds())
	decayEnd := currentTime + (time.Duration(8 * time.Second).Milliseconds())

	response, err := bidderClient.SendBid(txHashes, amount, blockNumberInt64, decayStart, decayEnd)
	if err != nil {
		log.Fatalf("Failed to send bid: %v", err)
	}

	fmt.Printf("Bid response: %v\n", response)

	// 7/10/24 - 0.4.0 release has auto deposits so this code is no longer needed.
	// After preconf bid is sent and confirmed, wait 11 minutes and then withdraw the funds.
	// time.Sleep(11 * time.Minute)

	// // Withdraw the amount from the window
	// err = bidderClient.WithdrawFunds(windowNumber)
	// if err != nil {
	// 	log.Fatalf("Failed to withdraw funds: %v", err)
	// }
}
