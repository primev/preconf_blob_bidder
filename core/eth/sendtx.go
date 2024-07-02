package eth

import (
	"bytes"
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

// send an eth transfer to self
func SelfSendETHTransfer(client *ethclient.Client, authAcct bb.AuthAcct, value *big.Int, gasLimit uint64, data []byte) (string, error) {
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

	// Set max priority fee per gas to twice the max fee per gas
	maxPriorityFee := new(big.Int).Mul(baseFee, big.NewInt(2))

	// Calculate max fee per gas
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
