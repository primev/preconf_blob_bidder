package mevcommit

import (
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
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
func WindowHeight(filePath string, client *ethclient.Client) (*big.Int, error) {
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
