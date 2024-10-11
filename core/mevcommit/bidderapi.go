// Package mevcommit provides functionality for interacting with the mev-commit protocol,
// including sending bids for blob transactions and saving bid requests and responses.
package mevcommit

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	pb "github.com/primev/preconf_blob_bidder/core/bidderpb"
)

func (b *Bidder) SendBid(input interface{}, amount string, blockNumber, decayStart, decayEnd int64) (pb.Bidder_SendBidClient, error) {
	// Prepare variables to hold transaction hashes or raw transactions
	var txHashes []string
	var rawTransactions []string

	// Determine the input type and process accordingly
	switch v := input.(type) {
	case []string:
		// If input is a slice of transaction hashes
		txHashes = make([]string, len(v))
		for i, hash := range v {
			txHashes[i] = strings.TrimPrefix(hash, "0x")
		}
	case []*types.Transaction:
		// If input is a slice of *types.Transaction, convert to raw transactions
		rawTransactions = make([]string, len(v))
		for i, tx := range v {
			rlpEncodedTx, err := tx.MarshalBinary()
			if err != nil {
				log.Error("Failed to marshal transaction to raw format", "error", err)
				return nil, fmt.Errorf("failed to marshal transaction: %w", err)
			}
			rawTransactions[i] = hex.EncodeToString(rlpEncodedTx)
		}
	default:
		log.Warn("Unsupported input type, must be []string or []*types.Transaction")
		return nil, fmt.Errorf("unsupported input type: %T", input)
	}

	// Create a new bid request with the appropriate transaction data
	bidRequest := &pb.Bid{
		Amount:              amount,
		BlockNumber:         blockNumber,
		DecayStartTimestamp: decayStart,
		DecayEndTimestamp:   decayEnd,
	}

	if len(txHashes) > 0 {
		bidRequest.TxHashes = txHashes
	} else if len(rawTransactions) > 0 {
		// Convert rawTransactions to []string
		rawTxStrings := make([]string, len(rawTransactions))
		for i, rawTx := range rawTransactions {
			rawTxStrings[i] = string(rawTx)
		}
		bidRequest.RawTransactions = rawTxStrings
	}

	ctx := context.Background()

	// Send the bid request to the mev-commit client
	response, err := b.client.SendBid(ctx, bidRequest)
	if err != nil {
		log.Error("Failed to send bid", "error", err)
		return nil, fmt.Errorf("failed to send bid: %w", err)
	}

	var responses []interface{}
	submitTimestamp := time.Now().Unix()

	// Save the bid request along with the submission timestamp
	go saveBidRequest("data/bid.json", bidRequest, submitTimestamp)

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
