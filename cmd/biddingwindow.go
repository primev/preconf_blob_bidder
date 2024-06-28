package main

import (
	"fmt"
	"log"
	"time"

	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

func main() {
	cfg := bb.Config{
		ServerAddress: "localhost:13524", // Default address for mevcommit gRPC server
		LogFmt:        "json",            // Example log format
		LogLevel:      "info",            // Example log level
	}

	// Print the start time
	fmt.Println("Start time: ", time.Now())

	// New mevcommit bidder node client connection
	bidderClient, err := bb.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Get the minimum deposit
	response, err := bidderClient.GetMinDeposit()
	if err != nil {
		log.Fatalf("Failed to get minimum deposit: %v", err)
	}
	fmt.Printf("Minimum deposit required: %v\n", response.Amount)

	fmt.Println("End time: ", time.Now())
}
