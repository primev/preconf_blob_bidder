package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

var NUM_BLOBS = 6

func main() {
	cfg := bb.BidderConfig{
		ServerAddress: "127.0.0.1:13524",
		LogFmt:        "json",
		LogLevel:      "info",
	}

	bidderClient, err := bb.NewBidderClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v. Remember to connect to the mev-commit p2p bidder node.", err)
	}
	fmt.Println("Connected to mev-commit client")

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

	timer := time.NewTimer(12 * time.Hour)
	blobCount := 0
	pendingTxs := make(map[string]int64)
	preconfCount := make(map[string]int)

	for {
		select {
		case <-timer.C:
			fmt.Println("2 hours have passed. Stopping the loop.")
			return
		default:
			if len(pendingTxs) == 0 {
				authAcct, err := bb.AuthenticateAddress(*privateKeyHex, client)
				if err != nil {
					log.Fatalf("Failed to authenticate private key: %v", err)
				}

				txHash, err := ee.ExecuteBlobTransaction(client, *endpoint, *private, *authAcct, NUM_BLOBS)
				if err != nil {
					log.Fatalf("Failed to execute blob transaction: %v", err)
				}

				blockNumber, err := client.BlockNumber(context.Background())
				if err != nil {
					log.Fatalf("Failed to retrieve block number: %v", err)
				}

				// log.Printf("Sent tx %s at block number: %d", txHash, blockNumber)

				pendingTxs[txHash] = int64(blockNumber)
				preconfCount[txHash] = 1
				blobCount++
				log.Printf("Number of blobs sent: %d", blobCount)

				// Send initial preconfirmation bid
				sendPreconfBid(bidderClient, txHash, int64(blockNumber)+1)
			} else {
				// Check pending transactions and resend preconfirmation bids if necessary
				checkPendingTxs(client, bidderClient, pendingTxs, preconfCount)
			}

			time.Sleep(3 * time.Second)
		}
	}
}

func sendPreconfBid(bidderClient *bb.Bidder, txHash string, blockNumber int64) {
	currentTime := time.Now().UnixMilli()
	amount := "250000000000000" // amount is in wei. Equivalent to .00025 ETH bids
	decayStart := currentTime
	decayEnd := currentTime + (time.Duration(12 * time.Second).Milliseconds()) // bid decay is 24 seconds (2 blocks)

	_, err := bidderClient.SendBid([]string{strings.TrimPrefix(txHash, "0x")}, amount, blockNumber, decayStart, decayEnd)
	if err != nil {
		log.Printf("Failed to send bid: %v", err)
	} else {
		log.Printf("Sent preconfirmation bid for tx: %s for block number: %d", txHash, blockNumber)
	}
}

func checkPendingTxs(client *ethclient.Client, bidderClient *bb.Bidder, pendingTxs map[string]int64, preconfCount map[string]int) {
	for txHash, initialBlock := range pendingTxs {
		receipt, err := client.TransactionReceipt(context.Background(), common.HexToHash(txHash))
		if err != nil {
			if err == ethereum.NotFound {
				// Transaction is still pending, resend preconfirmation bid
				currentBlockNumber, err := client.BlockNumber(context.Background())
				if err != nil {
					log.Printf("Failed to retrieve current block number: %v", err)
					continue
				}
				if currentBlockNumber > uint64(initialBlock) {
					sendPreconfBid(bidderClient, txHash, int64(currentBlockNumber)+1)
					preconfCount[txHash]++
					log.Printf("Resent preconfirmation bid for tx: %s in block number: %d. Total preconfirmations: %d", txHash, currentBlockNumber, preconfCount[txHash])
				}
			} else {
				log.Printf("Error checking transaction receipt: %v", err)
			}
		} else {
			// Transaction is confirmed, remove from pendingTxs
			delete(pendingTxs, txHash)
			log.Printf("Transaction %s confirmed in block %d, initially sent in block %d. Total preconfirmations: %d", txHash, receipt.BlockNumber.Uint64(), initialBlock, preconfCount[txHash])
			delete(preconfCount, txHash)
		}
	}
}
