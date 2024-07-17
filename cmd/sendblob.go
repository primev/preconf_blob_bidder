package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

func main() {

	// Start mevcommit bidder node client
	cfg := bb.BidderConfig{
		ServerAddress: "localhost:13524", // Default address for mevcommit gRPC server
		LogFmt:        "json",            // Example log format
		LogLevel:      "info",            // Example log level
	}

	bidderClient, err := bb.NewBidderClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v. Remember to connect to the mev-commit p2p bidder node.", err)
	}
	fmt.Println("Connected to mev-commit client")

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

	// Execute Blob Transaction
	txHash, err := ee.ExecuteBlobTransaction(client, *authAcct, 2)
	if err != nil {
		log.Fatalf("Failed to execute blob transaction: %v", err)
	}

	log.Printf("tx sent: %s", txHash)

	blockNumberInt64 := int64(blockNumber) + 1
	fmt.Printf("Preconf block number: %v\n", blockNumberInt64)
	currentTime := time.Now().UnixMilli()

	// Use a WaitGroup to wait for the goroutine to finish
	var wg sync.WaitGroup
	wg.Add(1)

	// Send preconf bid using a goroutine
	go func() {
		defer wg.Done()

		// Print sending preconf
		fmt.Println("Sending preconf bid...")
		txHashes := []string{strings.TrimPrefix(txHash, "0x")}
		amount := "1000000000000" // Specify amount in wei
		decayStart := currentTime - (time.Duration(8 * time.Second).Milliseconds())
		decayEnd := currentTime + (time.Duration(8 * time.Second).Milliseconds())

		response, err := bidderClient.SendBid(txHashes, amount, blockNumberInt64, decayStart, decayEnd)
		if err != nil {
			log.Fatalf("Failed to send bid: %v", err)
		}

		for {
			msg, err := response.Recv()
			if err == io.EOF {
				// End of stream
				break
			}
			if err != nil {
				log.Printf("Failed to receive bid response: %v", err)
				return
			}

			// Print the received message properly
			fmt.Printf("Bid sent successfully: %+v\n", msg)
		}
	}()

	// Wait for the goroutine to finish
	wg.Wait()
}
