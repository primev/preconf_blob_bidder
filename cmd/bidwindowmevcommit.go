package main

import (
	"context"
	"fmt"
	"time"

	"flag"
	"log"

	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

// run with go run cmd/bidwindowmevcommit.go --privatekey "private key" --endpoint "endpoint"
// This script mimics the same bidder functionality in the mev-commit bidder API, but calling the smart contracts
// directly using Geth. The minimum bid amount is retrieved from the blockTracker contract and used as the default
// deposit amount. Once the amount is deposited, the script calls `getDeposit` to confirm the deposit.

// The script needs to wait about 12 minutes before the funds are available to be withdrawn. This is an overestimation. Not sure why.
// Each window is 10 blocks, so about 120 seconds. After 360 seconds, or 6 minutes it should be good to withdraw from the window.
// The oracle lag also needs to be taken into account, which lags behind by 20 blocks.

func biddingWindow() {
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")
	privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	flag.Parse()
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	client, err := bb.NewGethClient(*endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to MEV-Commit chain: %v", err)
	}

	// Get block number for mev-commit
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("mev-commit Block Number: ", blockNumber)

	// Get current bidding window
	currentWindow, err := bb.WindowHeight(client)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("Current Bidding Window: ", currentWindow)

	// Authenticate address
	authAcct, err := bb.AuthenticateAddress(*privateKeyHex, client)
	if err != nil {
		log.Fatalf("Failed to Authenticate private key: %v", err)
	}

	tx, err := bb.DepositIntoWindow(client, currentWindow, authAcct)
	if err != nil {
		log.Fatalf("Failed to deposit into window: %v", err)
	}

	fmt.Printf("Transaction sent: %s\n", tx.Hash().Hex())

	depositAmount, err := bb.GetDepositAmount(client, authAcct.Address, *currentWindow)
	if err != nil {
		log.Fatalf("Failed to get deposit amount: %v", err)
	}
	fmt.Printf("The address %s deposited in window %d the amount %d\n", authAcct.Address, currentWindow, depositAmount)

	// Wait for 11 minutes before withdrawing. This is an overestimated time to ensure that the next window has started.
	log.Println("Waiting for 11 minutes before withdrawing...")
	time.Sleep(11 * time.Minute)

	// PART 3: WITHDRAW FUNDS
	// withdraw funds
	// withdrawBidderAmountFromWindow(address payable bidder,uint256 window)

	withdrawalTx, err := bb.WithdrawFromWindow(client, authAcct, currentWindow)
	if err != nil {
		log.Fatalf("Failed to withdraw funds: %v", err)
	}
	fmt.Printf("Withdrawal Transaction sent: %s\n", withdrawalTx.Hash().Hex())

}
