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

// example that gets the minimum bidder deposit from the gRPC bidderapi.
// TODO - add with API
func main() {

	// Start Holesky client
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	flag.Parse()
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	// Start mev-commit bidder Client
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
	txHash, err := ee.SelfSendETHTransfer(client, *authAcct, big.NewInt(100000), 3000000, []byte{0x4c, 0xdc, 0xeb, 0x20})
	if err != nil {
		log.Fatalf("Failed to send transaction: %v", err)
	}

	log.Printf("tx sent: %s", txHash)

	// mevcommit bidder node client
	cfg := bb.BidderConfig{
		ServerAddress: "localhost:13524", // Default address for mevcommit gRPC server
		LogFmt:        "json",            // Example log format
		LogLevel:      "info",            // Example log level
	}

	bidderClient, err := bb.NewBidderClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Get the minimum deposit
	response, err := bidderClient.GetMinDeposit()
	if err != nil {
		log.Fatalf("Failed to get minimum deposit: %v", err)
	}
	fmt.Printf("Minimum deposit required: %v\n", response.Amount)

	// Deposit minimum amount
	windowNumber, err := bidderClient.DepositMinBidAmount()
	if err != nil {
		log.Fatalf("Failed to deposit minimum bid amount: %v", err)
	}

	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to retrieve block number: %v", err)
	}

	// Convert uint64 to int64. Add +1 to be the next block number
	blockNumberInt64 := int64(blockNumber)
	// print current block number
	fmt.Printf("Current block number: %v\n", blockNumberInt64)
	currentTime := time.Now().UnixMilli()
	// preconf parameters
	txHashes := []string{strings.TrimPrefix(txHash, "0x")}
	amount := "1000000000" // Specify amount in wei
	decayStart := currentTime - (time.Duration(8 * time.Second).Milliseconds())
	decayEnd := currentTime + (time.Duration(8 * time.Second).Milliseconds())

	// send bid
	bidderClient.SendBid(txHashes, amount, blockNumberInt64, decayStart, decayEnd)

	// After preconf bid is sent and confirmed, wait 11 minutes and then withdraw the funds.

	// Wait 11 minutes
	time.Sleep(11 * time.Minute)

	// Withdraw the amount from the window
	err = bidderClient.WithdrawFunds(windowNumber)
	if err != nil {
		log.Fatalf("Failed to withdraw funds: %v", err)
	}
}
