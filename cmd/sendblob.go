package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
	"golang.org/x/exp/rand"
)

var NUM_BLOBS = 6
var MAX_PRECONF_ATTEMPTS = 50
var RECONNECT_INTERVAL = 30 * time.Second // Interval to wait before attempting to reconnect
var MAX_RPC_RETRIES = 5                   // Max retries for RPC endpoint
var RPC_TIMEOUT = 30 * time.Second        // Timeout for RPC calls

func main() {
	rpcEndpoints := flag.String("rpc-endpoints", "", "Comma-separated list of Ethereum client endpoints")
	wsEndpoint := flag.String("ws-endpoint", "", "The Ethereum client WebSocket endpoint")
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	offset := flag.Uint64("offset", 1, "Number of blocks to delay the transaction")
	usePayload := flag.Bool("use-payload", false, "Set to true to send transactions using payload instead of transaction hashes")

	glogger := log.NewGlogHandler(log.NewTerminalHandler(os.Stderr, true))
	glogger.Verbosity(log.LevelInfo)
	log.SetDefault(log.NewLogger(glogger))

	flag.Parse()
	if *rpcEndpoints == "" {
		log.Crit("use the rpc-endpoints flag to provide it.", "err", errors.New("endpoints are required"))
	}

	if *wsEndpoint == "" {
		log.Crit("use the ws-endpoint flag to provide it.", "err", errors.New("endpoint is required"))
	}

	authAcct, err := bb.AuthenticateAddress(*privateKeyHex)
	if err != nil {
		log.Crit("Failed to authenticate private key:", "err", err)
	}

	bidderAddress := os.Getenv("BIDDER_ADDRESS")
	if bidderAddress == "" {
		bidderAddress = "127.0.0.1:13524"
	}

	cfg := bb.BidderConfig{
		ServerAddress: bidderAddress,
		LogFmt:        "json",
		LogLevel:      "info",
	}

	bidderClient, err := bb.NewBidderClient(cfg)
	if err != nil {
		log.Crit("failed to create bidder client, remember to connect to the mev-commit p2p bidder node.", "err", err)
	}

	log.Info("connected to mev-commit client")

	// Split the RPC endpoints and connect to each
	rpcEndpointsList := strings.Split(*rpcEndpoints, ",")
	var rpcClients []*ethclient.Client
	for _, endpoint := range rpcEndpointsList {
		client := connectRPCClientWithRetries(endpoint, MAX_RPC_RETRIES, RPC_TIMEOUT)
		if client == nil {
			log.Error("failed to connect to RPC client, skipping endpoint", "endpoint", endpoint)
			continue
		}
		rpcClients = append(rpcClients, client)
		log.Info("(rpc) geth client connected", "endpoint", endpoint)
	}

	// Initial WebSocket connection
	wsClient, err := connectWSClient(*wsEndpoint)
	if err != nil {
		log.Crit("failed to connect to geth client", "err", err)
	}
	log.Info("(ws) geth client connected")

	headers := make(chan *types.Header)
	sub, err := wsClient.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		log.Crit("failed to subscribe to new blocks", "err", err)
	}

	timer := time.NewTimer(24 * 14 * time.Hour)
	pendingTxs := make(map[string]int64)
	preconfCount := make(map[string]int)

	for {
		select {
		case <-timer.C:
			log.Info("Stopping the loop.")
			return
		case err := <-sub.Err():
			log.Warn("subscription error", "err", err)
			wsClient, sub = reconnectWSClient(*wsEndpoint, headers)
			continue
		case header := <-headers:
			log.Info("new block generated", "block", header.Number)
			if len(pendingTxs) == 0 {
				for _, rpcEndpoint := range rpcEndpointsList {
					// Check if the RPC client is valid
					if rpcEndpoint == "" {
						log.Warn("Skipping empty RPC endpoint")
						continue
					}
					signedTx, blockNumber, err := ee.ExecuteBlobTransaction(wsClient, rpcEndpoint, header, authAcct, NUM_BLOBS, *offset)
					log.Info("Transaction fee values",
						"GasTipCap", signedTx.GasTipCap(),
						"GasFeeCap", signedTx.GasFeeCap(),
						"GasLimit", signedTx.Gas(),
						"BlobFeeCap", signedTx.BlobGasFeeCap(),
					)
					if *usePayload {
						// If use-payload is true, send the transaction payload to mev-commit. Don't send bundle
						sendPreconfBid(bidderClient, signedTx, int64(blockNumber))
					} else {
						_, err = ee.SendBundle(rpcEndpoint, signedTx, blockNumber)
						if err != nil {
							log.Error("Failed to send transaction", "rpcEndpoint", rpcEndpoint, "error", err)
						}
						sendPreconfBid(bidderClient, signedTx.Hash().String(), int64(blockNumber))
					}

					// handle ExecuteBlob error
					if err != nil {
						log.Warn("failed to execute blob tx", "err", err)
						continue // Skip to the next endpoint
					}

				}
			} else {
				// Check pending transactions and resend preconfirmation bids if necessary
				checkPendingTxs(rpcClients, bidderClient, pendingTxs, preconfCount)
			}
		}
	}
}

// Function to connect to RPC client with retry logic and timeout
func connectRPCClientWithRetries(rpcEndpoint string, maxRetries int, timeout time.Duration) *ethclient.Client {
	var rpcClient *ethclient.Client
	var err error

	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		rpcClient, err = ethclient.DialContext(ctx, rpcEndpoint)
		if err == nil {
			return rpcClient
		}

		log.Warn("failed to connect to RPC client, retrying...", "attempt", i+1, "err", err)
		time.Sleep(RECONNECT_INTERVAL * time.Duration(math.Pow(2, float64(i)))) // Exponential backoff
	}

	log.Error("failed to connect to RPC client after retries", "err", err)
	return nil
}

func connectWSClient(wsEndpoint string) (*ethclient.Client, error) {
	wsClient, err := bb.NewGethClient(wsEndpoint)
	if err != nil {
		log.Warn("failed to connect to websocket client", "err", err)
		time.Sleep(RECONNECT_INTERVAL)
		return connectWSClient(wsEndpoint)
	}
	return wsClient, nil
}

// Reconnect function for WebSocket client
func reconnectWSClient(wsEndpoint string, headers chan *types.Header) (*ethclient.Client, ethereum.Subscription) {
	var wsClient *ethclient.Client
	var sub ethereum.Subscription
	var err error

	for i := 0; i < 10; i++ { // Retry logic for WebSocket connection
		wsClient, err = connectWSClient(wsEndpoint)
		if err == nil {
			log.Info("(ws) geth client reconnected")
			sub, err = wsClient.SubscribeNewHead(context.Background(), headers)
			if err == nil {
				return wsClient, sub
			}
		}
		log.Warn("failed to reconnect WebSocket client, retrying...", "attempt", i+1, "err", err)
		time.Sleep(RECONNECT_INTERVAL)
	}
	log.Crit("failed to reconnect WebSocket client after retries", "err", err)
	return nil, nil
}

// sendPreconfBid sends a preconfirmation bid to the bidder client for a specified transaction.
//
// Parameters:
//   - bidderClient (*bb.Bidder): The bidder client used to send the bid.
//   - input (interface{}): The input can either be a transaction hash (string) or a pointer to a types.Transaction object.
//   - blockNumber (int64): The block number at which the bid is valid.
//
// The function generates a random bid amount between 0.00001 and 0.05 ETH, converts it to wei, and sends the bid with a decay time window.
// If the input type is not supported, the function logs a warning and exits.
func sendPreconfBid(bidderClient *bb.Bidder, input interface{}, blockNumber int64) {
	// Seed the random number generator
	rand.Seed(uint64(time.Now().UnixNano()))

	// Generate a random number between 0.000005 and 0.0025 ETH
	minAmount := 0.000005
	maxAmount := 0.001
	randomEthAmount := minAmount + rand.Float64()*(maxAmount-minAmount)

	// Convert the random ETH amount to wei (1 ETH = 10^18 wei)
	randomWeiAmount := int64(randomEthAmount * 1e18)

	// Convert the amount to a string for the bidder
	amount := fmt.Sprintf("%d", randomWeiAmount)

	// Get current time in milliseconds
	currentTime := time.Now().UnixMilli()

	// Define bid decay start and end
	decayStart := currentTime
	decayEnd := currentTime + int64(time.Duration(36*time.Second).Milliseconds()) // bid decay is 36 seconds (2 blocks)

	// Determine how to handle the input
	var err error
	switch v := input.(type) {
	case string:
		// Input is a string, process it as a transaction hash
		txHash := strings.TrimPrefix(v, "0x")
		log.Info("sending bid with transaction hash", "tx", input)
		// Send the bid with tx hash string
		_, err = bidderClient.SendBid([]string{txHash}, amount, blockNumber, decayStart, decayEnd)

	case *types.Transaction:
		// Input is a transaction object, send the transaction object
		log.Info("sending bid with tx payload", "tx", input.(*types.Transaction).Hash().String())
		// Send the bid with the full transaction object
		_, err = bidderClient.SendBid([]*types.Transaction{v}, amount, blockNumber, decayStart, decayEnd)

	default:
		log.Warn("unsupported input type, must be string or *types.Transaction")
		return
	}

	if err != nil {
		log.Warn("failed to send bid", "err", err)
	} else {
		log.Info("sent preconfirmation bid", "block", blockNumber, "amount (ETH)", randomEthAmount)
	}
}

func checkPendingTxs(clients []*ethclient.Client, bidderClient *bb.Bidder, pendingTxs map[string]int64, preconfCount map[string]int) {
	for txHash, initialBlock := range pendingTxs {
		for _, client := range clients {
			if client == nil {
				continue // Skip nil clients
			}
			receipt, err := client.TransactionReceipt(context.Background(), common.HexToHash(txHash))
			if err != nil {
				if err == ethereum.NotFound {
					// Transaction is still pending, resend preconfirmation bid
					currentBlockNumber, err := client.BlockNumber(context.Background())
					if err != nil {
						log.Error("failed to retrieve current block number", "err", err)
						continue
					}
					if currentBlockNumber > uint64(initialBlock) {
						sendPreconfBid(bidderClient, txHash, int64(currentBlockNumber)+1)
						preconfCount[txHash]++

						log.Info("Resent preconfirmation bid for tx",
							"txHash", txHash,
							"block number", currentBlockNumber,
							"total preconfirmations", preconfCount[txHash])

						// Check if preconfCount exceeds MAX_PRECONF_ATTEMPTS
						if preconfCount[txHash] >= MAX_PRECONF_ATTEMPTS {
							log.Warn("Max preconfirmation attempts reached for tx. Restarting with a new transaction.",
								"txHash", txHash)
							delete(pendingTxs, txHash)
							delete(preconfCount, txHash)
						}
					}
				} else {
					log.Error("Error checking transaction receipt", "err", err)
				}
			} else {
				// Transaction is confirmed, remove from pendingTxs
				delete(pendingTxs, txHash)
				log.Info("Transaction confirmed",
					"txHash", txHash,
					"confirmed block", receipt.BlockNumber.Uint64(),
					"initially sent block", initialBlock,
					"total preconfirmations", preconfCount[txHash])
				delete(preconfCount, txHash)
			}
		}
	}
}
