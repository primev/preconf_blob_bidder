package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Evan-Kim2028/example_bidder_go/biddingwindow"
)

func main() {
	cfg := biddingwindow.Config{
		ServerAddress: "localhost:13524",
		LogFmt:        "text",
		LogLevel:      "info",
	}

	// Print the start time
	fmt.Println("Start time: ", time.Now())

	// Get the minimum deposit
	response, err := biddingwindow.GetMinDeposit(cfg)
	if err != nil {
		log.Fatalf("Failed to get minimum deposit: %v", err)
	}
	fmt.Printf("Minimum deposit required: %v\n", response.Amount)

	// Deposit the minimum amount and get the window number
	windowNumber, err := biddingwindow.DepositMinBidAmount(cfg)
	if err != nil {
		log.Fatalf("Failed to deposit minimum bid amount: %v", err)
	}
	fmt.Printf("Deposited into window number: %v\n", windowNumber)

	// Wait for 11 minutes before withdrawing the funds
	fmt.Println("Waiting for 11 minutes before withdrawing the funds...")
	time.Sleep(11 * time.Minute)

	// Withdraw the funds from the specified window number
	if err := biddingwindow.WithdrawFunds(cfg, windowNumber); err != nil {
		log.Fatalf("Failed to withdraw funds: %v", err)
	}

	fmt.Println("End time: ", time.Now())
}
