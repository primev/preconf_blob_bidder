package main

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/joho/godotenv"
	ee "github.com/primev/preconf_blob_bidder/core/eth"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
	"golang.org/x/exp/rand"
)


func main() {
	// Load the .env file
	err := godotenv.Load()
	if err != nil {
		log.Crit("Error loading .env file", "err", err)
	}

	// Set up logging
	glogger := log.NewGlogHandler(log.NewTerminalHandler(os.Stderr, true))
	glogger.Verbosity(log.LevelInfo)
	log.SetDefault(log.NewLogger(glogger))

	// Read configuration from environment variables
	bidderAddress := os.Getenv("BIDDER_ADDRESS")
	if bidderAddress == "" {
		bidderAddress = "mev-commit-bidder:13523"
	}

	// NOTE 10/10/24 the rpc endpoint is not being used anymore. 
	rpcEndpoint := os.Getenv("RPC_ENDPOINT")
	if rpcEndpoint == "" {
		log.Crit("RPC_ENDPOINT environment variable is required")
	}

	wsEndpoint := os.Getenv("WS_ENDPOINT")
	if wsEndpoint == "" {
		log.Crit("WS_ENDPOINT environment variable is required")
	}

	privateKeyHex := os.Getenv("PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Crit("PRIVATE_KEY environment variable is required")
	}

	offsetEnv := os.Getenv("OFFSET")
	var offset uint64 = 1 // Default offset
	if offsetEnv != "" {
		// Convert offsetEnv to uint64
		var err error
		offset, err = parseUintEnvVar("OFFSET", offsetEnv)
		if err != nil {
			log.Crit("Invalid OFFSET value", "err", err)
		}
	}

	usePayloadEnv := os.Getenv("USE_PAYLOAD")
	usePayload := false // Default value
	if usePayloadEnv != "" {
		// Convert usePayloadEnv to bool
		var err error
		usePayload, err = parseBoolEnvVar("USE_PAYLOAD", usePayloadEnv)
		if err != nil {
			log.Crit("Invalid USE_PAYLOAD value", "err", err)
		}
	}

	// Log configuration values (excluding sensitive data)
	log.Info("Configuration values",
		"bidderAddress", bidderAddress,
		"rpcEndpoint", rpcEndpoint,
		"wsEndpoint", wsEndpoint,
		"offset", offset,
		"usePayload", usePayload,
	)


	authAcct, err := bb.AuthenticateAddress(privateKeyHex)
	if err != nil {
		log.Crit("Failed to authenticate private key:", "err", err)
	}

	cfg := bb.BidderConfig{
		ServerAddress: bidderAddress,
		LogFmt:        "json",
		LogLevel:      "info",
	}

	bidderClient, err := bb.NewBidderClient(cfg)
	if err != nil {
		log.Crit("failed to connect to mev-commit bidder API", "err", err)
	}

	log.Info("connected to mev-commit client")

	timeout := 30 * time.Second
	
	// Connect to RPC client
	client := connectRPCClientWithRetries(rpcEndpoint, 5, timeout)
	if client == nil {
		log.Error("failed to connect to RPC client", rpcEndpoint)
	}
	log.Info("(rpc) geth client connected", "endpoint", rpcEndpoint)

	// Connect to WS client
	wsClient, err := connectWSClient(wsEndpoint)
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

	for {
		select {
		case <-timer.C:
			log.Info("Stopping the loop.")
			return
		case err := <-sub.Err():
			log.Warn("subscription error", "err", err)
			wsClient, sub = reconnectWSClient(wsEndpoint, headers)
			continue
		case header := <-headers:
			log.Info("new block generated", "block", header.Number)

			amount := new(big.Int).SetInt64(1e15)
			signedTx, blockNumber, err := ee.SelfETHTransfer(wsClient, authAcct, amount, offset)

			log.Info("Transaction fee values",
				"GasTipCap", signedTx.GasTipCap(),
				"GasFeeCap", signedTx.GasFeeCap(),
				"GasLimit", signedTx.Gas(),
				"txHash", signedTx.Hash().String(),
				"blockNumber", blockNumber,
				"payloadSize", len(signedTx.Data()))

			if usePayload {
				// If use-payload is true, send the transaction payload to mev-commit. Don't send bundle
				sendPreconfBid(bidderClient, signedTx, int64(blockNumber))
			} else {
			// send as a flashbots bundle and send the preconf bid with the transaction hash
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
		time.Sleep(10 * time.Duration(math.Pow(2, float64(i)))) // Exponential backoff
	}

	log.Error("failed to connect to RPC client after retries", "err", err)
	return nil
}

func connectWSClient(wsEndpoint string) (*ethclient.Client, error) {
	wsClient, err := bb.NewGethClient(wsEndpoint)
	if err != nil {
		log.Warn("failed to connect to websocket client", "err", err)
		// sleep for 10 seconds
		time.Sleep(10 * time.Second)
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
		time.Sleep(5 * time.Second)
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
	minAmount := 0.00005
	maxAmount := 0.009
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


// Helper function to parse bool environment variables
func parseBoolEnvVar(name, value string) (bool, error) {
	parsedValue, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("environment variable %s must be true or false, got '%s'", name, value)
	}
	return parsedValue, nil
}

// parseUintEnvVar parses a string environment variable into a uint64.
// It returns the parsed value or an error if the parsing fails.
func parseUintEnvVar(name, value string) (uint64, error) {
    parsedValue, err := strconv.ParseUint(value, 10, 64)
    if err != nil {
        return 0, fmt.Errorf("environment variable %s must be a positive integer, got '%s'", name, value)
    }
    return parsedValue, nil
}