// Package eth provides functionality for sending Ethereum transactions,
// including blob transactions with preconfirmation bids. This package
// is designed to work with public Ethereum nodes and a custom Titan
// endpoint for private transactions.
package eth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gokzg4844 "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

// SelfETHTransfer sends an ETH transfer to the sender's own address. This function only works with
// public RPC endpoints and does not work with custom Titan endpoints.
//
// Parameters:
// - client: The Ethereum client instance.
// - authAcct: The authenticated account struct containing the address and private key.
// - value: The amount of ETH to transfer (in wei).
// - gasLimit: The maximum amount of gas to use for the transaction.
// - data: Optional data to include with the transaction.
//
// Returns:
// - The transaction hash as a string, or an error if the transaction fails.
func SelfETHTransfer(client *ethclient.Client, authAcct bb.AuthAcct, value *big.Int, gasLimit uint64, data []byte) (string, error) {
	// Get the account's nonce
	nonce, err := client.PendingNonceAt(context.Background(), authAcct.Address)
	if err != nil {
		return "", err
	}

	// Get the current base fee per gas from the latest block header
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return "", err
	}
	baseFee := header.BaseFee

	// Set the max priority fee per gas to be 10 times the base fee
	maxPriorityFee := new(big.Int).Mul(baseFee, big.NewInt(10))

	// Set the max fee per gas to be 10 times the max priority fee
	maxFeePerGas := new(big.Int).Mul(maxPriorityFee, big.NewInt(10))

	// Get the chain ID (this does not work with the Titan RPC)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return "", err
	}

	// Create a new EIP-1559 transaction
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		To:        &authAcct.Address,
		Value:     value,
		Gas:       gasLimit,
		GasFeeCap: maxFeePerGas,
		GasTipCap: maxPriorityFee,
		Data:      data,
	})

	// Sign the transaction with the authenticated account's private key
	signer := types.LatestSignerForChainID(chainID)
	signedTx, err := types.SignTx(tx, signer, authAcct.PrivateKey)
	if err != nil {
		return "", err
	}

	// Encode the signed transaction into RLP format for transmission
	var buf bytes.Buffer
	err = signedTx.EncodeRLP(&buf)
	if err != nil {
		return "", err
	}

	// Send the signed transaction to the Ethereum network
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", err
	}

	return signedTx.Hash().Hex(), nil
}

var (
	chainID     *big.Int
	chainIDOnce sync.Once
	chainIDErr  error
)

func getChainID(client *ethclient.Client, ctx context.Context) (*big.Int, error) {
	chainIDOnce.Do(func() {
		chainID, chainIDErr = client.NetworkID(ctx)
	})
	return chainID, chainIDErr
}

// ExecuteBlobTransaction sends a signed blob transaction to the network. If the private flag is set to true,
// the transaction is sent only to the Titan endpoint. Otherwise, it is sent to the specified public RPC endpoint.
//
// Parameters:
// - client: The Ethereum client instance.
// - rpcEndpoint: The RPC endpoint URL to send the transaction to.
// - private: A flag indicating whether to send the transaction to the Titan endpoint only.
// - authAcct: The authenticated account struct containing the address and private key.
// - numBlobs: The number of blobs to include in the transaction.
//
// Returns:
// - The transaction hash as a string, or an error if the transaction fails.
func ExecuteBlobTransaction(client *ethclient.Client, rpcEndpoint string, parentHeader *types.Header, private bool, authAcct bb.AuthAcct, numBlobs int, offset uint64) (string, uint64, error) {
	privateKey := authAcct.PrivateKey
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", 0, errors.New("failed to cast public key to ECDSA")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	ctx := context.Background()

	var (
		gasLimit    = uint64(500_000)
		blockNumber uint64
		nonce       uint64
		gasTipCap   *big.Int
		gasFeeCap   *big.Int
		err1, err2  error
	)

	// Connect to the Titan Holesky client
	//	titan_client, err := bb.NewGethClient("http://holesky-rpc.titanbuilder.xyz/")
	//	if err != nil {
	//		fmt.Println("Failed to connect to titan client: ", err)
	//	}

	chainID, err := getChainID(client, context.Background())
	if err != nil {
		return "", 0, err
	}

	// Fetch various transaction parameters in parallel
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		nonce, err1 = client.PendingNonceAt(context.Background(), fromAddress)
	}()

	go func() {
		defer wg.Done()
		gasTipCap, gasFeeCap, err2 = suggestGasTipAndFeeCap(client, ctx)
	}()

	wg.Wait()
	if err1 != nil {
		return "", 0, err1
	}
	if err2 != nil {
		return "", 0, err2
	}

	log.Info("account nonce tracker", "nonce", nonce)
	blockNumber = parentHeader.Number.Uint64()

	// Calculate the blob fee cap and ensure it is sufficient for transaction replacement
	parentExcessBlobGas := eip4844.CalcExcessBlobGas(*parentHeader.ExcessBlobGas, *parentHeader.BlobGasUsed)
	blobFeeCap := eip4844.CalcBlobFee(parentExcessBlobGas)
	blobFeeCap.Add(blobFeeCap, big.NewInt(1)) // Ensure it's at least 1 unit higher to replace a transaction

	// Generate random blobs and their corresponding sidecar
	blobs := randBlobs(numBlobs)
	sideCar := makeSidecar(blobs)
	blobHashes := sideCar.BlobHashes()

	// Incrementally increase blob fee cap for replacement
	incrementFactor := big.NewInt(110) // 10% increase
	blobFeeCap.Mul(blobFeeCap, incrementFactor).Div(blobFeeCap, big.NewInt(100))

	// Adjust gas tip cap and fee cap incrementally
	//priorityFeeIncrement := big.NewInt(10000000) // 0.01 gwei increase
	priorityFeeIncrement := big.NewInt(20000000000) // 20 gwei increase
	gasTipCapAdjusted := new(big.Int).Add(gasTipCap, priorityFeeIncrement)

	// Ensure gasTipCapAdjusted doesn't exceed your max intended value (0.5 gwei)
	maxPriorityFee := new(big.Int).Mul(priorityFeeIncrement, big.NewInt(50)) // 0.5 gwei
	if gasTipCapAdjusted.Cmp(maxPriorityFee) > 0 {
		gasTipCapAdjusted.Set(maxPriorityFee)
	}

	// Ensure GasFeeCap is higher than GasTipCap
	gasFeeCapAdjusted := new(big.Int).Mul(gasTipCapAdjusted, big.NewInt(2))
	if gasFeeCap.Cmp(gasFeeCapAdjusted) <= 0 {
		gasFeeCapAdjusted.Add(gasFeeCapAdjusted, big.NewInt(1)) // Ensure it's higher
	}

	// Create a new BlobTx transaction
	tx := types.NewTx(&types.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      nonce,
		GasTipCap:  uint256.MustFromBig(gasTipCapAdjusted),
		GasFeeCap:  uint256.MustFromBig(gasFeeCapAdjusted),
		Gas:        gasLimit,
		To:         fromAddress,
		BlobFeeCap: uint256.MustFromBig(blobFeeCap),
		BlobHashes: blobHashes,
		Sidecar:    sideCar,
	})

	// Sign the transaction with the authenticated account's private key
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return "", 0, err
	}

	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return "", 0, err
	}

	retryAttempts := 5
	for i := 0; i < retryAttempts; i++ {
		if private {
			// Send the transaction only to the Titan endpoint
			//err = titan_client.SendTransaction(ctx, signedTx)
			_, err = sendBundle("http://holesky-rpc.titanbuilder.xyz/", signedTx, blockNumber+offset)
		} else {
			// Send the transaction to the specified public RPC endpoint
			//err = client.SendTransaction(ctx, signedTx)
			_, err = sendBundle(rpcEndpoint, signedTx, blockNumber+offset)
		}

		if err != nil && strings.Contains(err.Error(), "replacement transaction underpriced") {
			// Increment gas fee cap slightly and try again
			incrementFactor := big.NewInt(105) // 105% increase
			gasFeeCapAdjusted.Mul(gasFeeCapAdjusted, incrementFactor).Div(gasFeeCapAdjusted, big.NewInt(100))

			// Recreate and sign the transaction with updated gasFeeCap
			tx = types.NewTx(&types.BlobTx{
				ChainID:    uint256.MustFromBig(chainID),
				Nonce:      nonce,
				GasTipCap:  uint256.MustFromBig(gasTipCapAdjusted),
				GasFeeCap:  uint256.MustFromBig(gasFeeCapAdjusted),
				Gas:        gasLimit,
				To:         fromAddress,
				BlobFeeCap: uint256.MustFromBig(blobFeeCap),
				BlobHashes: blobHashes,
				Sidecar:    sideCar,
			})
			signedTx, err = auth.Signer(auth.From, tx)
			if err != nil {
				return "", 0, err
			}
		} else {
			break
		}
	}

	if err != nil {
		return "", 0, fmt.Errorf("failed to replace transaction after %d attempts: %v", retryAttempts, err)
	}

	// Record the transaction parameters and save them asynchronously
	currentTimeMillis := time.Now().UnixNano() / int64(time.Millisecond)

	transactionParameters := map[string]interface{}{
		"hash":          signedTx.Hash().String(),
		"chainID":       signedTx.ChainId(),
		"nonce":         signedTx.Nonce(),
		"gasTipCap":     signedTx.GasTipCap(),
		"gasFeeCap":     signedTx.GasFeeCap(),
		"gasLimit":      signedTx.Gas(),
		"to":            signedTx.To(),
		"blobFeeCap":    signedTx.BlobGasFeeCap(),
		"blobHashes":    signedTx.BlobHashes(),
		"timeSubmitted": currentTimeMillis,
		"numBlobs":      numBlobs,
	}

	go saveTransactionParameters("data/blobs.json", transactionParameters) // Asynchronous saving

	return signedTx.Hash().String(), blockNumber + offset, nil
}

// suggestGasTipAndFeeCap suggests a gas tip cap and gas fee cap for a transaction, ensuring that the values
// are sufficient for timely inclusion in the next block.
//
// Parameters:
// - client: The Ethereum client instance.
// - ctx: The context for making requests to the Ethereum client.
//
// Returns:
// - The suggested gas tip cap and gas fee cap as big.Int pointers, or an error if the suggestions fail.
//func suggestGasTipAndFeeCap(client *ethclient.Client, ctx context.Context) (*big.Int, *big.Int, error) {
//	gasTipCap, err := client.SuggestGasTipCap(ctx)
//	if err != nil {
//		return nil, nil, err
//	}
//
//	minGasTipCap := big.NewInt(1000000000) // 1 Gwei minimum gas tip cap
//	if gasTipCap.Cmp(minGasTipCap) < 0 {
//		gasTipCap = minGasTipCap
//	}
//
//	gasFeeCap, err := client.SuggestGasPrice(ctx)
//	if err != nil {
//		return nil, nil, err
//	}
//
//	buffer := big.NewInt(1000000000) // 1 Gwei buffer to ensure gas fee cap is higher than gas tip cap
//	if gasFeeCap.Cmp(new(big.Int).Add(gasTipCap, buffer)) < 0 {
//		gasFeeCap = new(big.Int).Add(gasTipCap, buffer)
//	}
//
//	return gasTipCap, gasFeeCap, nil
//}

// suggestGasTipAndFeeCap suggests a gas tip cap and gas fee cap for a transaction, ensuring that the values
// are sufficient for timely inclusion in the next block.
//
// Parameters:
// - client: The Ethereum client instance.
// - ctx: The context for making requests to the Ethereum client.
//
// Returns:
// - The suggested gas tip cap and gas fee cap as big.Int pointers, or an error if the suggestions fail.
func suggestGasTipAndFeeCap(client *ethclient.Client, ctx context.Context) (*big.Int, *big.Int, error) {
	var (
		gasTipCap, gasFeeCap *big.Int
		err1, err2           error
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		gasTipCap, err1 = client.SuggestGasTipCap(ctx)
	}()

	go func() {
		defer wg.Done()
		gasFeeCap, err2 = client.SuggestGasPrice(ctx)
	}()

	wg.Wait()

	if err1 != nil {
		return nil, nil, err1
	}
	if err2 != nil {
		return nil, nil, err2
	}

	return gasTipCap, gasFeeCap, nil
}

// sendPrivateRawTransaction sends a signed transaction directly to the Titan endpoint as a private transaction.
//
// Parameters:
// - rpcEndpoint: The RPC endpoint URL to send the transaction to.
// - signedTx: The signed transaction to be sent.
//
// Returns:
// - An error if the transaction fails to send.
func sendPrivateRawTransaction(rpcEndpoint string, signedTx *types.Transaction) error {
	// Marshal the signed transaction to binary format
	binary, err := signedTx.MarshalBinary()
	if err != nil {
		log.Error("Error marshaling transaction", "error", err)
		return fmt.Errorf("error marshaling transaction: %v", err)
	}

	// Prepare the JSON-RPC payload
	method := "POST"
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_sendPrivateRawTransaction",
		"params": []string{
			"0x" + common.Bytes2Hex(binary),
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Error("Error marshaling payload", "error", err)
		return fmt.Errorf("error marshaling payload: %v", err)
	}

	// Send the HTTP request to the Titan endpoint
	httpClient := &http.Client{}
	req, err := http.NewRequest(method, rpcEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Error("Error creating request", "error", err)
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error("Error sending request", "error", err)
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read and log the response from the Titan endpoint
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error("Error reading response body", "error", err)
		return fmt.Errorf("error reading response body: %v", err)
	}
	log.Info("Response private transaction", "body", string(body))

	return nil
}

// saveTransactionParameters saves transaction parameters to a JSON file, appending them to an existing array of transactions.
//
// Parameters:
// - filename: The name of the JSON file to save the transaction parameters to.
// - params: The transaction parameters to save as a map of string keys to interface{} values.
func saveTransactionParameters(filename string, params map[string]interface{}) {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Failed to create directory", "directory", dir, "error", err)
		return
	}

	var transactions []map[string]interface{}

	// Open the file and decode any existing transactions
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file", "filename", filename, "error", err)
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&transactions); err != nil && err.Error() != "EOF" {
		log.Error("Failed to decode existing JSON data", "error", err)
		return
	}

	// Append the new transaction parameters
	transactions = append(transactions, params)

	// Write the updated transactions array to the file
	file.Seek(0, 0)  // Move to the beginning of the file
	file.Truncate(0) // Clear the file content

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(transactions); err != nil {
		log.Error("Failed to encode parameters to JSON", "error", err)
	}
}

// makeSidecar creates a sidecar for the given blobs, including commitments and proofs.
//
// Parameters:
// - blobs: A slice of kzg4844.Blob objects.
//
// Returns:
// - A pointer to a types.BlobTxSidecar containing the blobs, commitments, and proofs.
func makeSidecar(blobs []kzg4844.Blob) *types.BlobTxSidecar {
	var (
		commitments []kzg4844.Commitment
		proofs      []kzg4844.Proof
	)

	// Generate commitments and proofs for each blob
	for _, blob := range blobs {
		c, _ := kzg4844.BlobToCommitment(&blob)
		p, _ := kzg4844.ComputeBlobProof(&blob, c)

		commitments = append(commitments, c)
		proofs = append(proofs, p)
	}

	return &types.BlobTxSidecar{
		Blobs:       blobs,
		Commitments: commitments,
		Proofs:      proofs,
	}
}

// randBlobs generates a slice of random blobs.
//
// Parameters:
// - n: The number of blobs to generate.
//
// Returns:
// - A slice of randomly generated blobs.
func randBlobs(n int) []kzg4844.Blob {
	blobs := make([]kzg4844.Blob, n)
	for i := 0; i < n; i++ {
		blobs[i] = randBlob()
	}
	return blobs
}

// randBlob generates a single random blob.
//
// Returns:
// - A randomly generated blob.
func randBlob() kzg4844.Blob {
	var blob kzg4844.Blob
	for i := 0; i < len(blob); i += gokzg4844.SerializedScalarSize {
		fieldElementBytes := randFieldElement()
		copy(blob[i:i+gokzg4844.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}

// randFieldElement generates a random field element for use in blob generation.
//
// Returns:
// - A 32-byte array representing a random field element.
func randFieldElement() [32]byte {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		panic("failed to get random field element")
	}
	var r fr.Element
	r.SetBytes(bytes)

	return gokzg4844.SerializeScalar(r)
}
