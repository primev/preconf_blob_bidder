package eth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gokzg4844 "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum"
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

// send an eth transfer to self
func SelfETHTransfer(client *ethclient.Client, authAcct bb.AuthAcct, value *big.Int, gasLimit uint64, data []byte) (string, error) {
	// Get Address nonce
	nonce, err := client.PendingNonceAt(context.Background(), authAcct.Address)
	if err != nil {
		return "", err
	}

	// Get base fee per gas
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return "", err
	}
	baseFee := header.BaseFee

	// Set max priority fee per gas as twice the base fee
	maxPriorityFee := new(big.Int).Mul(baseFee, big.NewInt(2))

	// Calculate max fee per gas as twice the max priority fee
	maxFeePerGas := new(big.Int).Mul(maxPriorityFee, big.NewInt(2))

	// Get chainID
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return "", err
	}

	// Create EIP-1559 transaction
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		To:        &authAcct.Address,
		Value:     value,
		Gas:       gasLimit,
		GasFeeCap: maxFeePerGas,
		GasTipCap: maxPriorityFee,
		Data:      data,
	})

	signer := types.LatestSignerForChainID(chainID)
	signedTx, err := types.SignTx(tx, signer, authAcct.PrivateKey)
	if err != nil {
		return "", err
	}

	// Encode the signed transaction into RLP (Recursive Length Prefix) format for transmission.
	var buf bytes.Buffer
	err = signedTx.EncodeRLP(&buf)
	if err != nil {
		return "", err
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return "", err
	}

	return signedTx.Hash().Hex(), nil
}

func ExecuteBlobTransaction(client *ethclient.Client, rpcEndpoint string, private bool, authAcct bb.AuthAcct, numBlobs int) (string, error) {
	glogger := log.NewGlogHandler(log.NewTerminalHandler(os.Stderr, true))
	glogger.Verbosity(log.LevelInfo)
	log.SetDefault(log.NewLogger(glogger))

	privateKey := authAcct.PrivateKey
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("failed to cast public key to ECDSA")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	ctx := context.Background()

	var (
		chainID                *big.Int
		nonce                  uint64
		gasTipCap              *big.Int
		gasFeeCap              *big.Int
		parentHeader           *types.Header
		err1, err2, err3, err4 error
	)

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		chainID, err1 = client.NetworkID(ctx)
	}()

	go func() {
		defer wg.Done()
		nonce, err2 = client.PendingNonceAt(ctx, fromAddress)
	}()

	go func() {
		defer wg.Done()
		gasTipCap, gasFeeCap, err3 = suggestGasTipAndFeeCap(client, ctx)
	}()

	go func() {
		defer wg.Done()
		parentHeader, err4 = client.HeaderByNumber(ctx, nil)
	}()

	wg.Wait()
	if err1 != nil {
		return "", err1
	}
	if err2 != nil {
		return "", err2
	}
	if err3 != nil {
		return "", err3
	}
	if err4 != nil {
		return "", err4
	}

	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From:      fromAddress,
		To:        &fromAddress,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Value:     big.NewInt(0),
	})
	if err != nil {
		return "", err
	}

	parentExcessBlobGas := eip4844.CalcExcessBlobGas(*parentHeader.ExcessBlobGas, *parentHeader.BlobGasUsed)
	blobFeeCap := eip4844.CalcBlobFee(parentExcessBlobGas)

	log.Info("Blob gas info",
		"excessBlobGas", parentExcessBlobGas,
		"blobFeeCap", blobFeeCap)

	blobs := randBlobs(numBlobs)
	sideCar := makeSidecar(blobs)
	blobHashes := sideCar.BlobHashes()

	tx := types.NewTx(&types.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      nonce,
		GasTipCap:  uint256.MustFromBig(gasTipCap),
		GasFeeCap:  uint256.MustFromBig(gasFeeCap),
		Gas:        gasLimit * 120 / 10,
		To:         fromAddress,
		BlobFeeCap: uint256.MustFromBig(blobFeeCap),
		BlobHashes: blobHashes,
		Sidecar:    sideCar,
	})

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return "", err
	}

	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return "", err
	}

	if private {
		err = sendPrivateRawTransaction(rpcEndpoint, signedTx)
		if err != nil {
			return "", err
		}
	} else {
		err = client.SendTransaction(ctx, signedTx)
		if err != nil {
			return "", err
		}
	}

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

	log.Info("Transaction parameters",
		"hash", signedTx.Hash().String(),
		"chainID", signedTx.ChainId(),
		"nonce", signedTx.Nonce(),
		"gasTipCap", signedTx.GasTipCap(),
		"gasFeeCap", signedTx.GasFeeCap(),
		"gasLimit", signedTx.Gas(),
		"to", signedTx.To(),
		"blobFeeCap", signedTx.BlobGasFeeCap(),
		"blobHashes", signedTx.BlobHashes(),
		"timeSubmitted", currentTimeMillis,
		"numBlobs", numBlobs)

	go saveTransactionParameters("data/blobs.json", transactionParameters) // Asynchronous saving

	return signedTx.Hash().String(), nil
}

func suggestGasTipAndFeeCap(client *ethclient.Client, ctx context.Context) (*big.Int, *big.Int, error) {
	gasTipCap, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, err
	}

	minGasTipCap := big.NewInt(1000000000) // 1 Gwei
	if gasTipCap.Cmp(minGasTipCap) < 0 {
		gasTipCap = minGasTipCap
	}

	gasFeeCap, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, nil, err
	}

	buffer := big.NewInt(1000000000) // 1 Gwei buffer
	if gasFeeCap.Cmp(new(big.Int).Add(gasTipCap, buffer)) < 0 {
		gasFeeCap = new(big.Int).Add(gasTipCap, buffer)
	}

	return gasTipCap, gasFeeCap, nil
}

func sendPrivateRawTransaction(rpcEndpoint string, signedTx *types.Transaction) error {
	binary, err := signedTx.MarshalBinary()
	if err != nil {
		log.Error("Error marshaling transaction", "error", err)
		return fmt.Errorf("error marshaling transaction: %v", err)
	}

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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error("Error reading response body", "error", err)
		return fmt.Errorf("error reading response body: %v", err)
	}
	log.Info("Response private transaction", "body", string(body))

	return nil
}

// saveTransactionParameters saves transaction parameters to a JSON file
func saveTransactionParameters(filename string, params map[string]interface{}) {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Failed to create directory", "directory", dir, "error", err)
		return
	}

	var transactions []map[string]interface{}

	// Read existing file content
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

func makeSidecar(blobs []kzg4844.Blob) *types.BlobTxSidecar {
	var (
		commitments []kzg4844.Commitment
		proofs      []kzg4844.Proof
	)

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

func randBlobs(n int) []kzg4844.Blob {
	blobs := make([]kzg4844.Blob, n)
	for i := 0; i < n; i++ {
		blobs[i] = randBlob()
	}
	return blobs
}

func randBlob() kzg4844.Blob {
	var blob kzg4844.Blob
	for i := 0; i < len(blob); i += gokzg4844.SerializedScalarSize {
		fieldElementBytes := randFieldElement()
		copy(blob[i:i+gokzg4844.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}

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
