// Package mevcommit provides functionality for interacting with the mev-commit protocol,
// including sending bids for blob transactions and saving bid requests and responses.
package mevcommit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/log"
	pb "github.com/primev/preconf_blob_bidder/core/bidderpb"
)

// SendBid sends a bid to the mev-commit client for a given set of transaction hashes, amount, and block number.
// The bid will be decayed over a specified time range.
//
// Parameters:
// - txHashes: A slice of transaction hashes to bid on.
// - amount: The bid amount in wei as a string.
// - blockNumber: The block number for which the bid applies.
// - decayStart: The start timestamp for bid decay (in milliseconds).
// - decayEnd: The end timestamp for bid decay (in milliseconds).
//
// Returns:
// - A pb.Bidder_SendBidClient to receive bid responses, or an error if the bid fails.
func (b *Bidder) SendBid(txHashes []string, amount string, blockNumber, decayStart, decayEnd int64) (pb.Bidder_SendBidClient, error) {
	// Initialize logger
	glogger := log.NewGlogHandler(log.NewTerminalHandler(os.Stderr, true))
	glogger.Verbosity(log.LevelInfo)
	log.SetDefault(log.NewLogger(glogger))

	// Create a new bid request
	bidRequest := &pb.Bid{
		TxHashes:            txHashes,
		Amount:              amount,
		BlockNumber:         blockNumber,
		DecayStartTimestamp: decayStart,
		DecayEndTimestamp:   decayEnd,
	}

	ctx := context.Background()

	// Timer before creating context
	startTimeBeforeContext := time.Now()

	// Send the bid request to the mev-commit client
	response, err := b.client.SendBid(ctx, bidRequest)
	endTime := time.Since(startTimeBeforeContext).Milliseconds()
	fmt.Println("Time taken to send bid:", endTime)
	if err != nil {
		log.Error("Failed to send bid", "error", err)
		return nil, fmt.Errorf("failed to send bid: %w", err)
	}

	var responses []interface{}
	submitTimestamp := time.Now().Unix()

	// Save the bid request along with the submission timestamp
	saveBidRequest("data/bid.json", bidRequest, submitTimestamp)

	// Continuously receive bid responses
	for {
		msg, err := response.Recv()
		if err == io.EOF {
			// End of stream
			break
		}
		if err != nil {
			log.Error("Failed to receive bid response", "error", err)
			return nil, fmt.Errorf("failed to send bid: %w", err)
		}

		log.Info("Bid accepted", "commitment details", msg)
		responses = append(responses, msg)
	}

	// Timer before saving bid responses
	startTimeBeforeSaveResponses := time.Now()
	log.Info("End Time", "time", startTimeBeforeSaveResponses)

	// Save all bid responses to a file
	go saveBidResponses("data/response.json", responses)
	return response, nil
}

// saveBidRequest saves the bid request and timestamp to a JSON file.
// The data is appended to an array of existing bid requests.
//
// Parameters:
// - filename: The name of the JSON file to save the bid request to.
// - bidRequest: The bid request to save.
// - timestamp: The timestamp of when the bid was submitted (in Unix time).
func saveBidRequest(filename string, bidRequest *pb.Bid, timestamp int64) {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Failed to create directory", "directory", dir, "error", err)
		return
	}

	// Prepare the data to be saved
	data := map[string]interface{}{
		"timestamp":  timestamp,
		"bidRequest": bidRequest,
	}

	// Open the file, creating it if it doesn't exist
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file", "filename", filename, "error", err)
		return
	}
	defer file.Close()

	// Read existing data from the file
	var existingData []map[string]interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&existingData); err != nil && err.Error() != "EOF" {
		log.Error("Failed to decode existing JSON data", "error", err)
		return
	}

	// Append the new bid request to the existing data
	existingData = append(existingData, data)

	// Write the updated data back to the file
	file.Seek(0, 0)  // Move to the beginning of the file
	file.Truncate(0) // Clear the file content
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(existingData); err != nil {
		log.Error("Failed to encode data to JSON", "error", err)
	}
}

// saveBidResponses saves the bid responses to a JSON file.
// The responses are appended to an array of existing responses.
//
// Parameters:
// - filename: The name of the JSON file to save the bid responses to.
// - responses: A slice of bid responses to save.
func saveBidResponses(filename string, responses []interface{}) {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Failed to create directory", "directory", dir, "error", err)
		return
	}

	// Open the file, creating it if it doesn't exist
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file", "filename", filename, "error", err)
		return
	}
	defer file.Close()

	// Read existing data from the file
	var existingData []interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&existingData); err != nil && err.Error() != "EOF" {
		log.Error("Failed to decode existing JSON data", "error", err)
		return
	}

	// Append the new bid responses to the existing data
	existingData = append(existingData, responses...)

	// Write the updated responses back to the file
	file.Seek(0, 0)  // Move to the beginning of the file
	file.Truncate(0) // Clear the file content
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(existingData); err != nil {
		log.Error("Failed to encode data to JSON", "error", err)
	}
}
