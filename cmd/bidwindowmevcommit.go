package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

// contract addresses
const bidderRegistryAddress = "0x7ffa86fF89489Bca72Fec2a978e33f9870B2Bd25"
const blockTrackerAddress = "0x2eEbF31f5c932D51556E70235FB98bB2237d065c"

// This script mimics the same bidder functionality in the mev-commit bidder API, but calling the smart contracts
// directly using Geth. The minimum bid amount is retrieved from the blockTracker contract and used as the default
// deposit amount. Once the amount is deposited, the script calls `getDeposit` to confirm the deposit.

// The script needs to wait about 12 minutes before the funds are available to be withdrawn. This is an overestimation. Not sure why.
// Each window is 10 blocks, so about 120 seconds. After 360 seconds, or 6 minutes it should be good to withdraw from the window.
// The oracle lag also needs to be taken into account, which lags behind by 20 blocks.

func main() {
	// Define command-line flags for the private key and endpoint
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")

	// Parse the command-line flags
	flag.Parse()

	// Ensure that the private key is provided
	if *privateKeyHex == "" {
		log.Fatal("Private key is required. Use the -privatekey flag to provide it.")
	}

	// Ensure that the endpoint is provided
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	// NewMevCommitClient
	client, err := ethclient.Dial(*endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Get the private key
	privateKey, err := crypto.HexToECDSA(*privateKeyHex)
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}

	// Get the public key
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("Failed to assert public key type")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	// Print the public key
	log.Println("Public Key: ", fromAddress.Hex())

	// Create an auth transactor
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		log.Fatalf("Failed to create authorized transactor: %v", err)
	}

	// Get block number for mev-commit
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Println(err)
		return
	}
	// Current mevcommit block number
	log.Println("Block Number: ", blockNumber)

	// Load blockTracker contract
	blockTrackerABI, err := bb.LoadABI("abi/BlockTracker.abi")
	if err != nil {
		log.Println("Failed to load ABI file:", err)
		return
	}

	blockTrackerParsedABI, err := abi.JSON(strings.NewReader(blockTrackerABI))
	if err != nil {
		log.Println("Failed to load ABI file:", err)
		return
	}
	blockTrackerContract := bind.NewBoundContract(common.HexToAddress(blockTrackerAddress), blockTrackerParsedABI, client, client, client)

	// PART 1: Get current bidding window
	var currentWindowResult []interface{}
	err = blockTrackerContract.Call(nil, &currentWindowResult, "getCurrentWindow")
	if err != nil {
		log.Println(err)
		return
	}

	// Extract the current window as *big.Int
	currentWindow, ok := currentWindowResult[0].(*big.Int)
	if !ok {
		log.Println("Failed to convert current window to *big.Int")
		return
	}
	log.Println("Current Bidding Window: ", currentWindow)

	// load bidderRegistry contract
	bidderRegistryABI, err := bb.LoadABI("abi/BidderRegistry.abi")
	if err != nil {
		log.Println("Failed to load ABI file:", err)
		return
	}

	bidderRegistryParsedABI, err := abi.JSON(strings.NewReader(bidderRegistryABI))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	bidderRegistryContract := bind.NewBoundContract(common.HexToAddress(bidderRegistryAddress), bidderRegistryParsedABI, client, client, client)

	// PART 2: DEPOSIT INTO BIDDING CONTRACT
	// Call the minDeposit function
	var minDepositResult []interface{}
	err = bidderRegistryContract.Call(nil, &minDepositResult, "minDeposit")
	if err != nil {
		log.Println("Failed to call minDeposit function: ", err)
		return
	}

	// Extract the minDeposit as *big.Int
	minDeposit, ok := minDepositResult[0].(*big.Int)
	if !ok {
		log.Println("Failed to convert minDeposit to *big.Int")
		return
	}

	log.Println("Min Deposit: ", minDeposit)

	// Set the value to minDeposit
	auth.Value = minDeposit

	// Prepare the transaction
	tx, err := bidderRegistryContract.Transact(auth, "depositForSpecificWindow", currentWindow)
	if err != nil {
		log.Fatalf("Failed to create transaction: %v", err)
	}

	fmt.Printf("Transaction sent: %s\n", tx.Hash().Hex())

	// Wait for the transaction to be mined (optional)
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		log.Fatalf("Transaction mining error: %v", err)
	}

	if receipt.Status == 1 {
		fmt.Println("Transaction successful")
	} else {
		fmt.Println("Transaction failed")
	}

	// PART 2.5: Confirm bidder deposit
	// getDeposit(address bidder,uint256 window)
	var depositResult []interface{}
	err = bidderRegistryContract.Call(nil, &depositResult, "getDeposit", fromAddress, currentWindow)
	if err != nil {
		log.Fatalf("Failed to call getDeposit function: %v", err)
	}

	// Extract the deposit amount as *big.Int
	depositAmount, ok := depositResult[0].(*big.Int)
	if !ok {
		log.Fatalf("Failed to convert deposit amount to *big.Int")
	}

	fmt.Printf("Deposit Amount: %s\n", depositAmount.String())

	// Wait for 11 minutes before withdrawing. This is an overestimated time to ensure that the next window has started.
	log.Println("Waiting for 11 minutes before withdrawing...")
	time.Sleep(11 * time.Minute)

	// PART 3: WITHDRAW FUNDS
	// withdraw funds
	// withdrawBidderAmountFromWindow(address payable bidder,uint256 window)
	withdrawAmount := big.NewInt(183269)
	withdrawalTx, err := bidderRegistryContract.Transact(auth, "withdrawBidderAmountFromWindow", fromAddress, withdrawAmount)
	if err != nil {
		log.Fatalf("Failed to create withdrawal transaction: %v", err)
	}

	fmt.Printf("Withdrawal Transaction sent: %s\n", withdrawalTx.Hash().Hex())

	// Wait for the withdrawal transaction to be mined
	withdrawalReceipt, err := bind.WaitMined(context.Background(), client, withdrawalTx)
	if err != nil {
		log.Fatalf("Withdrawal transaction mining error: %v", err)
	}

	if withdrawalReceipt.Status == 1 {
		fmt.Println("Withdrawal successful")
	} else {
		fmt.Println("Withdrawal failed")
	}
}
