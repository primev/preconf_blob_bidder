package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

func main() {
	// Start mevcommit bidder node client
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

	// Start Holesky client with command line flags
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	private := flag.Bool("private", false, "Set to true for private transactions")

	flag.Parse()
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	client, err := bb.NewGethClient(*endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to client: %v", err)
	}

	// Create a timer for 1 hour
	timer := time.NewTimer(2 * time.Hour)
	blobCount := 0
	for {
		select {
		case <-timer.C:
			fmt.Println("1 hour has passed. Stopping the loop.")
			return
		default:
			// Authenticate address with private key
			authAcct, err := bb.AuthenticateAddress(*privateKeyHex, client)
			if err != nil {
				log.Fatalf("Failed to authenticate private key: %v", err)
			}

			// Execute Blob Transaction
			txHash, err := ee.ExecuteBlobTransaction(client, *endpoint, *private, *authAcct, 2)
			if err != nil {
				log.Fatalf("Failed to execute blob transaction: %v", err)
			}

			log.Printf("tx sent: %s", txHash)

			// Get current block number
			blockNumber, err := client.BlockNumber(context.Background())
			if err != nil {
				log.Fatalf("Failed to retrieve block number: %v", err)
			}
			fmt.Printf("Current block number: %v\n", blockNumber)

			blockNumberInt64 := int64(blockNumber) + 1
			fmt.Printf("Preconf block number: %v\n", blockNumberInt64)
			currentTime := time.Now().UnixMilli()

			// Send preconf bid
			txHashes := []string{strings.TrimPrefix(txHash, "0x")}
			amount := "25000000000000000" // amount is in wei. Equivalent to .025 ETH bids
			decayStart := currentTime     //- (time.Duration(1 * time.Millisecond).Milliseconds())
			decayEnd := currentTime + (time.Duration(8 * time.Second).Milliseconds())

			_, err = bidderClient.SendBid(txHashes, amount, blockNumberInt64, decayStart, decayEnd)
			if err != nil {
				log.Fatalf("Failed to send bid: %v", err)
			}
			blobCount++
			log.Printf("Number of blobs sent: %d", blobCount)

			// Wait for 45 seconds before sending the next transaction
			time.Sleep(45 * time.Second)
		}
	}
}
