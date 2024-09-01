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
	private := flag.Bool("private", false, "Set to true for private transactions")
	offset := flag.Uint64("offset", 1, "Number of blocks to delay the transaction")

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

	cfg := bb.BidderConfig{
		ServerAddress: "127.0.0.1:13524",
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
	blobCount := 0
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
					// Execute the transaction using wsClient for nonce and gas information
					txHash, blockNumber, err := ee.ExecuteBlobTransaction(wsClient, rpcEndpoint, header, *private, authAcct, NUM_BLOBS, *offset)
					if err != nil {
						log.Warn("failed to execute blob tx", "err", err)
					} else {
						preconfCount[txHash] = 1
						blobCount++
						log.Info("blobs sent", "count", blobCount, "tx", txHash, "block", blockNumber)
						// Send initial preconfirmation bid
						sendPreconfBid(bidderClient, txHash, int64(blockNumber))
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

	log.Crit("failed to connect to RPC client after retries", "err", err)
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

func sendPreconfBid(bidderClient *bb.Bidder, txHash string, blockNumber int64) {
	// Seed the random number generator
	rand.Seed(uint64(time.Now().UnixNano()))

	// Generate a random number between 0.00001 and 0.05 ETH
	minAmount := 0.00001
	maxAmount := 0.05
	randomEthAmount := minAmount + rand.Float64()*(maxAmount-minAmount)

	// Convert the random ETH amount to wei (1 ETH = 10^18 wei)
	randomWeiAmount := int64(randomEthAmount * 1e18)

	// Convert the amount to a string for the bidder
	amount := fmt.Sprintf("%d", randomWeiAmount)

	// Get current time in milliseconds
	currentTime := time.Now().UnixMilli()

	// Define bid decay start and end
	decayStart := currentTime
	decayEnd := currentTime + (time.Duration(36 * time.Second).Milliseconds()) // bid decay is 24 seconds (2 blocks)

	// Send the bid
	_, err := bidderClient.SendBid([]string{strings.TrimPrefix(txHash, "0x")}, amount, blockNumber, decayStart, decayEnd)
	if err != nil {
		log.Warn("failed to send bid", "err", err)
	} else {
		log.Info("sent preconfirmation bid", "tx", txHash, "block", blockNumber, "amount (ETH)", randomEthAmount)
	}
}

func checkPendingTxs(clients []*ethclient.Client, bidderClient *bb.Bidder, pendingTxs map[string]int64, preconfCount map[string]int) {
	for txHash, initialBlock := range pendingTxs {
		for _, client := range clients {
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

// // Package main provides functionality for sending Ethereum transactions,
// // including blob transactions with preconfirmation bids. This package
// // is designed to work with public Ethereum nodes and a custom Titan
// // endpoint for private transactions.
// package main

// import (
// 	"bytes"
// 	"context"
// 	"crypto/ecdsa"
// 	"crypto/rand"
// 	"encoding/json"
// 	"fmt"
// 	"io/ioutil"
// 	"math/big"
// 	"net/http"
// 	"os"
// 	"path/filepath"
// 	"sync"
// 	"time"

// 	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
// 	gokzg4844 "github.com/crate-crypto/go-kzg-4844"
// 	"github.com/ethereum/go-ethereum"
// 	"github.com/ethereum/go-ethereum/accounts/abi/bind"
// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
// 	"github.com/ethereum/go-ethereum/core/types"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/ethereum/go-ethereum/crypto/kzg4844"
// 	"github.com/ethereum/go-ethereum/ethclient"
// 	"github.com/ethereum/go-ethereum/log"
// 	"github.com/holiman/uint256"
// 	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
// )

// // ExecuteBlobTransaction sends a signed blob transaction to the network. If the private flag is set to true,
// // the transaction is sent only to the Titan endpoint. Otherwise, it is sent to the specified public RPC endpoint.
// //
// // Parameters:
// // - client: The Ethereum client instance.
// // - rpcEndpoint: The RPC endpoint URL to send the transaction to.
// // - private: A flag indicating whether to send the transaction to the Titan endpoint only.
// // - authAcct: The authenticated account struct containing the address and private key.
// // - numBlobs: The number of blobs to include in the transaction.
// //
// // Returns:
// // - The transaction hash as a string, or an error if the transaction fails.
// func ExecuteBlobTransaction(client *ethclient.Client, rpcEndpoint string, private bool, authAcct bb.AuthAcct, numBlobs int) (string, error) {
// 	// Initialize logger
// 	glogger := log.NewGlogHandler(log.NewTerminalHandler(os.Stderr, true))
// 	glogger.Verbosity(log.LevelInfo)
// 	log.SetDefault(log.NewLogger(glogger))

// 	privateKey := authAcct.PrivateKey
// 	publicKey := privateKey.Public()
// 	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
// 	if !ok {
// 		return "", fmt.Errorf("failed to cast public key to ECDSA")
// 	}
// 	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

// 	ctx := context.Background()

// 	var (
// 		chainID                *big.Int
// 		nonce                  uint64
// 		gasTipCap              *big.Int
// 		gasFeeCap              *big.Int
// 		parentHeader           *types.Header
// 		err1, err2, err3, err4 error
// 	)

// 	// Connect to the Titan Holesky client
// 	titan_client, err := bb.NewGethClient("http://holesky-rpc.titanbuilder.xyz/")
// 	if err != nil {
// 		fmt.Println("Failed to connect to titan client: ", err)
// 	}

// 	// Fetch the latest block number
// 	var blockNumber uint64
// 	blockNumber, err = client.BlockNumber(ctx)
// 	if err != nil {
// 		return "cant fetch latest block number", err
// 	}

// 	// Fetch various transaction parameters in parallel
// 	var wg sync.WaitGroup
// 	wg.Add(4)

// 	go func() {
// 		defer wg.Done()
// 		chainID, err1 = client.NetworkID(ctx)
// 	}()

// 	go func() {
// 		defer wg.Done()
// 		nonce, err2 = client.NonceAt(ctx, fromAddress, new(big.Int).SetUint64(blockNumber))
// 	}()

// 	go func() {
// 		defer wg.Done()
// 		gasTipCap, gasFeeCap, err3 = suggestGasTipAndFeeCap(client, ctx)
// 	}()

// 	go func() {
// 		defer wg.Done()
// 		parentHeader, err4 = client.HeaderByNumber(ctx, nil)
// 	}()

// 	wg.Wait()
// 	if err1 != nil {
// 		return "", err1
// 	}
// 	if err2 != nil {
// 		return "", err2
// 	}
// 	if err3 != nil {
// 		return "", err3
// 	}
// 	if err4 != nil {
// 		return "", err4
// 	}

// 	// Estimate the gas limit for the transaction
// 	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{
// 		From:      fromAddress,
// 		To:        &fromAddress,
// 		GasFeeCap: gasFeeCap,
// 		GasTipCap: gasTipCap,
// 		Value:     big.NewInt(0),
// 	})
// 	if err != nil {
// 		return "", err
// 	}

// 	// Calculate the blob fee cap and ensure it is sufficient for transaction replacement
// 	parentExcessBlobGas := eip4844.CalcExcessBlobGas(*parentHeader.ExcessBlobGas, *parentHeader.BlobGasUsed)
// 	blobFeeCap := eip4844.CalcBlobFee(parentExcessBlobGas)
// 	blobFeeCap.Add(blobFeeCap, big.NewInt(1)) // Ensure it's at least 1 unit higher to replace a transaction

// 	// Generate random blobs and their corresponding sidecar
// 	blobs := randBlobs(numBlobs)
// 	sideCar := makeSidecar(blobs)
// 	blobHashes := sideCar.BlobHashes()

// 	// Increase the blob fee cap to ensure replacement
// 	incrementFactor := big.NewInt(200) // 100% increase (double the fee cap)
// 	blobFeeCap.Mul(blobFeeCap, incrementFactor).Div(blobFeeCap, big.NewInt(100))

// 	fixed_priority_fee := big.NewInt(2000000000) // 2 gwei
// 	gasTipCapAdjusted := new(big.Int).Mul(fixed_priority_fee, big.NewInt(5))
// 	gasTipCapAdjusted.Add(gasTipCapAdjusted, big.NewInt(10000000000))

// 	// Calculate the replacement penalty for GasTipCap
// 	queuedGasTipCap := big.NewInt(100000000000) // Example value; replace with actual queued transaction's gas tip cap
// 	replacementTipPenalty := big.NewInt(2)      // 100% penalty (double the tip)

// 	newGasTipCap := new(big.Int).Mul(queuedGasTipCap, replacementTipPenalty)
// 	if gasTipCapAdjusted.Cmp(newGasTipCap) <= 0 {
// 		gasTipCapAdjusted.Set(newGasTipCap) // Ensure the new tip cap meets the replacement requirement
// 	}

// 	// Ensure GasFeeCap is higher than GasTipCap
// 	gasFeeCapAdjusted := new(big.Int).Mul(gasTipCapAdjusted, big.NewInt(2))
// 	if gasFeeCap.Cmp(gasFeeCapAdjusted) > 0 {
// 		gasFeeCapAdjusted.Set(gasFeeCap) // Use the original gasFeeCap if it's already larger
// 	}

// 	// Create a new BlobTx transaction
// 	tx := types.NewTx(&types.BlobTx{
// 		ChainID:    uint256.MustFromBig(chainID),
// 		Nonce:      nonce,
// 		GasTipCap:  uint256.MustFromBig(gasTipCapAdjusted),
// 		GasFeeCap:  uint256.MustFromBig(gasFeeCapAdjusted),
// 		Gas:        gasLimit * 120 / 10,
// 		To:         fromAddress,
// 		BlobFeeCap: uint256.MustFromBig(blobFeeCap),
// 		BlobHashes: blobHashes,
// 		Sidecar:    sideCar,
// 	})

// 	// Sign the transaction with the authenticated account's private key
// 	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
// 	if err != nil {
// 		return "", err
// 	}

// 	signedTx, err := auth.Signer(auth.From, tx)
// 	if err != nil {
// 		return "", err
// 	}

// 	if private {
// 		// Send the transaction only to the Titan endpoint
// 		err = titan_client.SendTransaction(ctx, signedTx)
// 		if err != nil {
// 			return "", err
// 		}
// 	} else {
// 		// Send the transaction to the specified public RPC endpoint
// 		err = client.SendTransaction(ctx, signedTx)
// 		if err != nil {
// 			return "", err
// 		}
// 	}

// 	// Record the transaction parameters and save them asynchronously
// 	currentTimeMillis := time.Now().UnixNano() / int64(time.Millisecond)

// 	transactionParameters := map[string]interface{}{
// 		"hash":          signedTx.Hash().String(),
// 		"chainID":       signedTx.ChainId(),
// 		"nonce":         signedTx.Nonce(),
// 		"gasTipCap":     signedTx.GasTipCap(),
// 		"gasFeeCap":     signedTx.GasFeeCap(),
// 		"gasLimit":      signedTx.Gas(),
// 		"to":            signedTx.To(),
// 		"blobFeeCap":    signedTx.BlobGasFeeCap(),
// 		"blobHashes":    signedTx.BlobHashes(),
// 		"timeSubmitted": currentTimeMillis,
// 		"numBlobs":      numBlobs,
// 	}

// 	go saveTransactionParameters("data/blobs.json", transactionParameters) // Asynchronous saving

// 	return signedTx.Hash().String(), nil
// }

// // suggestGasTipAndFeeCap suggests a gas tip cap and gas fee cap for a transaction, ensuring that the values
// // are sufficient for timely inclusion in the next block.
// //
// // Parameters:
// // - client: The Ethereum client instance.
// // - ctx: The context for making requests to the Ethereum client.
// //
// // Returns:
// // - The suggested gas tip cap and gas fee cap as big.Int pointers, or an error if the suggestions fail.
// func suggestGasTipAndFeeCap(client *ethclient.Client, ctx context.Context) (*big.Int, *big.Int, error) {
// 	gasTipCap, err := client.SuggestGasTipCap(ctx)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	minGasTipCap := big.NewInt(1000000000) // 1 Gwei minimum gas tip cap
// 	if gasTipCap.Cmp(minGasTipCap) < 0 {
// 		gasTipCap = minGasTipCap
// 	}

// 	gasFeeCap, err := client.SuggestGasPrice(ctx)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	buffer := big.NewInt(1000000000) // 1 Gwei buffer to ensure gas fee cap is higher than gas tip cap
// 	if gasFeeCap.Cmp(new(big.Int).Add(gasTipCap, buffer)) < 0 {
// 		gasFeeCap = new(big.Int).Add(gasTipCap, buffer)
// 	}

// 	return gasTipCap, gasFeeCap, nil
// }

// // sendPrivateRawTransaction sends a signed transaction directly to the Titan endpoint as a private transaction.
// //
// // Parameters:
// // - rpcEndpoint: The RPC endpoint URL to send the transaction to.
// // - signedTx: The signed transaction to be sent.
// //
// // Returns:
// // - An error if the transaction fails to send.
// func sendPrivateRawTransaction(rpcEndpoint string, signedTx *types.Transaction) error {
// 	// Marshal the signed transaction to binary format
// 	binary, err := signedTx.MarshalBinary()
// 	if err != nil {
// 		log.Error("Error marshaling transaction", "error", err)
// 		return fmt.Errorf("error marshaling transaction: %v", err)
// 	}

// 	// Prepare the JSON-RPC payload
// 	method := "POST"
// 	payload := map[string]interface{}{
// 		"jsonrpc": "2.0",
// 		"id":      1,
// 		"method":  "eth_sendPrivateRawTransaction",
// 		"params": []string{
// 			"0x" + common.Bytes2Hex(binary),
// 		},
// 	}

// 	payloadBytes, err := json.Marshal(payload)
// 	if err != nil {
// 		log.Error("Error marshaling payload", "error", err)
// 		return fmt.Errorf("error marshaling payload: %v", err)
// 	}

// 	// Send the HTTP request to the Titan endpoint
// 	httpClient := &http.Client{}
// 	req, err := http.NewRequest(method, rpcEndpoint, bytes.NewBuffer(payloadBytes))
// 	if err != nil {
// 		log.Error("Error creating request", "error", err)
// 		return fmt.Errorf("error creating request: %v", err)
// 	}
// 	req.Header.Add("Content-Type", "application/json")

// 	resp, err := httpClient.Do(req)
// 	if err != nil {
// 		log.Error("Error sending request", "error", err)
// 		return fmt.Errorf("error sending request: %v", err)
// 	}
// 	defer resp.Body.Close()

// 	// Read and log the response from the Titan endpoint
// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		log.Error("Error reading response body", "error", err)
// 		return fmt.Errorf("error reading response body: %v", err)
// 	}
// 	log.Info("Response private transaction", "body", string(body))

// 	return nil
// }

// // saveTransactionParameters saves transaction parameters to a JSON file, appending them to an existing array of transactions.
// //
// // Parameters:
// // - filename: The name of the JSON file to save the transaction parameters to.
// // - params: The transaction parameters to save as a map of string keys to interface{} values.
// func saveTransactionParameters(filename string, params map[string]interface{}) {
// 	// Ensure the directory exists
// 	dir := filepath.Dir(filename)
// 	if err := os.MkdirAll(dir, 0755); err != nil {
// 		log.Error("Failed to create directory", "directory", dir, "error", err)
// 		return
// 	}

// 	var transactions []map[string]interface{}

// 	// Open the file and decode any existing transactions
// 	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
// 	if err != nil {
// 		log.Error("Failed to open file", "filename", filename, "error", err)
// 		return
// 	}
// 	defer file.Close()

// 	decoder := json.NewDecoder(file)
// 	if err := decoder.Decode(&transactions); err != nil && err.Error() != "EOF" {
// 		log.Error("Failed to decode existing JSON data", "error", err)
// 		return
// 	}

// 	// Append the new transaction parameters
// 	transactions = append(transactions, params)

// 	// Write the updated transactions array to the file
// 	file.Seek(0, 0)  // Move to the beginning of the file
// 	file.Truncate(0) // Clear the file content

// 	encoder := json.NewEncoder(file)
// 	if err := encoder.Encode(transactions); err != nil {
// 		log.Error("Failed to encode parameters to JSON", "error", err)
// 	}
// }

// // makeSidecar creates a sidecar for the given blobs, including commitments and proofs.
// //
// // Parameters:
// // - blobs: A slice of kzg4844.Blob objects.
// //
// // Returns:
// // - A pointer to a types.BlobTxSidecar containing the blobs, commitments, and proofs.
// func makeSidecar(blobs []kzg4844.Blob) *types.BlobTxSidecar {
// 	var (
// 		commitments []kzg4844.Commitment
// 		proofs      []kzg4844.Proof
// 	)

// 	// Generate commitments and proofs for each blob
// 	for _, blob := range blobs {
// 		c, _ := kzg4844.BlobToCommitment(&blob)
// 		p, _ := kzg4844.ComputeBlobProof(&blob, c)

// 		commitments = append(commitments, c)
// 		proofs = append(proofs, p)
// 	}

// 	return &types.BlobTxSidecar{
// 		Blobs:       blobs,
// 		Commitments: commitments,
// 		Proofs:      proofs,
// 	}
// }

// // randBlobs generates a slice of random blobs.
// //
// // Parameters:
// // - n: The number of blobs to generate.
// //
// // Returns:
// // - A slice of randomly generated blobs.
// func randBlobs(n int) []kzg4844.Blob {
// 	blobs := make([]kzg4844.Blob, n)
// 	for i := 0; i < n; i++ {
// 		blobs[i] = randBlob()
// 	}
// 	return blobs
// }

// // randBlob generates a single random blob.
// //
// // Returns:
// // - A randomly generated blob.
// func randBlob() kzg4844.Blob {
// 	var blob kzg4844.Blob
// 	for i := 0; i < len(blob); i += gokzg4844.SerializedScalarSize {
// 		fieldElementBytes := randFieldElement()
// 		copy(blob[i:i+gokzg4844.SerializedScalarSize], fieldElementBytes[:])
// 	}
// 	return blob
// }

// // randFieldElement generates a random field element for use in blob generation.
// //
// // Returns:
// // - A 32-byte array representing a random field element.
// func randFieldElement() [32]byte {
// 	bytes := make([]byte, 32)
// 	_, err := rand.Read(bytes)
// 	if err != nil {
// 		panic("failed to get random field element")
// 	}
// 	var r fr.Element
// 	r.SetBytes(bytes)

// 	return gokzg4844.SerializeScalar(r)
// }
