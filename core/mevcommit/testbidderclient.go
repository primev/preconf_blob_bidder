package mevcommit

import (
	"fmt"
	"testing"
	"time"
)

// tests an automated deposit and withdrawal of funds from the bidding window using the minimum deposit amount.
func TestBiddingWindow(t *testing.T) {
	cfg := Config{
		ServerAddress: "localhost:13524",
		LogFmt:        "text",
		LogLevel:      "info",
	}

	// Print the start time
	fmt.Println("Start time: ", time.Now())

	// Get the minimum deposit
	response, err := GetMinDeposit(cfg)
	if err != nil {
		t.Fatalf("Failed to get minimum deposit: %v", err)
	}
	fmt.Printf("Minimum deposit required: %v\n", response.Amount)

	// Deposit the minimum amount and get the window number
	windowNumber, err := DepositMinBidAmount(cfg)
	if err != nil {
		t.Fatalf("Failed to deposit minimum bid amount: %v", err)
	}
	fmt.Printf("Deposited into window number: %v\n", windowNumber)

	// Wait for 11 minutes before withdrawing the funds
	fmt.Println("Waiting for 11 minutes before withdrawing the funds...")
	time.Sleep(11 * time.Second) // Reduced for testing purposes

	// Withdraw the funds from the specified window number
	if err := WithdrawFunds(cfg, windowNumber); err != nil {
		t.Fatalf("Failed to withdraw funds: %v", err)
	}

	fmt.Println("End time: ", time.Now())
}
