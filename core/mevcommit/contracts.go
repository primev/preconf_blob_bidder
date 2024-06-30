package mevcommit

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// contract addresses
const bidderRegistryAddress = "0x7ffa86fF89489Bca72Fec2a978e33f9870B2Bd25"
const blockTrackerAddress = "0x2eEbF31f5c932D51556E70235FB98bB2237d065c"

// LoadABI loads the ABI from the specified file path and parses it
func LoadABI(filePath string) (abi.ABI, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Println("Failed to load ABI file:", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(string(data)))
	if err != nil {
		log.Println("Failed to load ABI file:", err)
	}

	return parsedABI, nil
}

// get latest window height
func WindowHeight(client *ethclient.Client) (*big.Int, error) {
	// Load blockTracker contract
	blockTrackerABI, err := LoadABI("abi/BlockTracker.abi")
	if err != nil {
		log.Println("Failed to load ABI file:", err)
	}

	blockTrackerContract := bind.NewBoundContract(common.HexToAddress(blockTrackerAddress), blockTrackerABI, client, client, client)

	// Get current bidding window
	var currentWindowResult []interface{}
	err = blockTrackerContract.Call(nil, &currentWindowResult, "getCurrentWindow")
	if err != nil {
		log.Println(err)
	}

	// Extract the current window as *big.Int
	currentWindow, ok := currentWindowResult[0].(*big.Int)
	if !ok {
		log.Println("Could not get current window", err)
	}

	return currentWindow, nil
}

func GetMinDeposit(client *ethclient.Client) (*big.Int, error) {
	bidderRegistryABI, err := LoadABI("abi/BidderRegistry.abi")
	if err != nil {
		return nil, fmt.Errorf("failed to load ABI file: %v", err)
	}

	bidderRegistryContract := bind.NewBoundContract(common.HexToAddress(bidderRegistryAddress), bidderRegistryABI, client, client, client)

	// Call the minDeposit function
	var minDepositResult []interface{}
	err = bidderRegistryContract.Call(nil, &minDepositResult, "minDeposit")
	if err != nil {
		return nil, fmt.Errorf("failed to call minDeposit function: %v", err)
	}

	// Extract the minDeposit as *big.Int
	minDeposit, ok := minDepositResult[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to convert minDeposit to *big.Int")
	}

	return minDeposit, nil
}

// Deposit minimum bid amount into the bidding window. Returns a geth Transaction type if successful.
func DepositIntoWindow(client *ethclient.Client, depositWindow *big.Int, authAcct *AuthAcct) (*types.Transaction, error) {
	// Load bidderRegistry contract
	bidderRegistryABI, err := LoadABI("abi/BidderRegistry.abi")
	if err != nil {
		return nil, fmt.Errorf("failed to load ABI file: %v", err)
	}

	bidderRegistryContract := bind.NewBoundContract(common.HexToAddress(bidderRegistryAddress), bidderRegistryABI, client, client, client)

	minDeposit, err := GetMinDeposit(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get minDeposit: %v", err)
	}

	// Set the value to minDeposit
	authAcct.Auth.Value = minDeposit

	// Prepare the transaction
	tx, err := bidderRegistryContract.Transact(authAcct.Auth, "depositForSpecificWindow", depositWindow)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %v", err)
	}

	// Wait for the transaction to be mined (optional)
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		return nil, fmt.Errorf("transaction mining error: %v", err)
	}

	if receipt.Status == 1 {
		fmt.Println("Transaction successful")
		return tx, nil
	} else {
		return nil, fmt.Errorf("transaction failed")
	}
}

// GetDepositAmount retrieves the deposit amount for a given address and window
func GetDepositAmount(client *ethclient.Client, address common.Address, window big.Int) (*big.Int, error) {
	bidderRegistryABI, err := LoadABI("abi/BidderRegistry.abi")
	if err != nil {
		return nil, fmt.Errorf("failed to load ABI file: %v", err)
	}

	bidderRegistryContract := bind.NewBoundContract(common.HexToAddress(bidderRegistryAddress), bidderRegistryABI, client, client, client)

	// Call the getDeposit function
	var depositResult []interface{}
	err = bidderRegistryContract.Call(nil, &depositResult, "minDeposit")
	if err != nil {
		return nil, fmt.Errorf("failed to call getDeposit function: %v", err)
	}

	// Extract the deposit amount as *big.Int
	depositAmount, ok := depositResult[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to convert deposit amount to *big.Int")
	}

	return depositAmount, nil
}

// WithdrawFromWindow withdraws all funds from the specified window
func WithdrawFromWindow(client *ethclient.Client, authAcct *AuthAcct, window *big.Int) (*types.Transaction, error) {
	// Load bidderRegistry contract
	bidderRegistryABI, err := LoadABI("abi/BidderRegistry.abi")
	if err != nil {
		return nil, fmt.Errorf("failed to load ABI file: %v", err)
	}

	bidderRegistryContract := bind.NewBoundContract(common.HexToAddress(bidderRegistryAddress), bidderRegistryABI, client, client, client)

	// Prepare the withdrawal transaction
	withdrawalTx, err := bidderRegistryContract.Transact(authAcct.Auth, "withdrawBidderAmountFromWindow", authAcct.Address, window)
	if err != nil {
		return nil, fmt.Errorf("failed to create withdrawal transaction: %v", err)
	}

	// Wait for the withdrawal transaction to be mined
	withdrawalReceipt, err := bind.WaitMined(context.Background(), client, withdrawalTx)
	if err != nil {
		return nil, fmt.Errorf("withdrawal transaction mining error: %v", err)
	}

	if withdrawalReceipt.Status == 1 {
		fmt.Println("Withdrawal successful")
		return withdrawalTx, nil
	} else {
		return nil, fmt.Errorf("withdrawal failed")
	}
}
