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
	pb "github.com/primev/mev-commit/p2p/gen/go/bidderapi/v1"
)

// GetMinDeposit retrieves the minimum deposit required for bidding using mev-commit bidder api.
func (b *Bidder) GetMinDeposit() (*pb.DepositResponse, error) {
	ctx := context.Background()
	response, err := b.client.GetMinDeposit(ctx, &pb.EmptyMessage{})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// DepositMinBidAmount deposits the minimum bid amount into the current bidding window
// TODO - add in window_number parameter as optional Uint64. If the value is not passed in, then the function will automatically use the latest window.
// TODO - add in block_number parameter as optional Uint64. It will calculate the window based on the block number.
func (b *Bidder) DepositMinBidAmount() (int64, error) {
	minDepositResponse, err := b.GetMinDeposit()
	if err != nil {
		return 0, fmt.Errorf("failed to get minimum deposit: %w", err)
	}

	minDepositAmount := minDepositResponse.Amount
	depositRequest := &pb.DepositRequest{
		Amount: minDepositAmount,
	}

	ctx := context.Background()
	response, err := b.client.Deposit(ctx, depositRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to deposit funds: %w", err)
	}

	windowNumber := int64(response.WindowNumber.Value)
	return windowNumber, nil
}

func (b *Bidder) SendBid(txHashes []string, amount string, blockNumber, decayStart, decayEnd int64) (pb.Bidder_SendBidClient, error) {
	glogger := log.NewGlogHandler(log.NewTerminalHandler(os.Stderr, true))
	glogger.Verbosity(log.LevelInfo)
	log.SetDefault(log.NewLogger(glogger))

	bidRequest := &pb.Bid{
		TxHashes:            txHashes,
		Amount:              amount,
		BlockNumber:         blockNumber,
		DecayStartTimestamp: decayStart,
		DecayEndTimestamp:   decayEnd,
	}

	log.Info("Sending bid request", "txHashes", txHashes, "amount", amount, "blockNumber", blockNumber, "decayStart", decayStart, "decayEnd", decayEnd)

	// Timer before creating context
	startTimeBeforeContext := time.Now()
	log.Info("Start time: ", startTimeBeforeContext)

	ctx := context.Background()

	response, err := b.client.SendBid(ctx, bidRequest)
	if err != nil {
		log.Error("Failed to send bid", "error", err)
		return nil, fmt.Errorf("failed to send bid: %w", err)
	}

	var responses []interface{}
	submitTimestamp := time.Now().Unix()
	saveBidRequest("data/bid.json", bidRequest, submitTimestamp)

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

		log.Info("Bid sent successfully", "response", msg)
		responses = append(responses, msg)
	}

	// Timer before saving bid responses
	startTimeBeforeSaveResponses := time.Now()
	log.Info("End Time", "startTimeBeforeSaveResponses", startTimeBeforeSaveResponses)

	saveBidResponses("data/response.json", responses)

	return response, nil
}

// saveBidRequest saves bid request and timestamp to a JSON file
func saveBidRequest(filename string, bidRequest *pb.Bid, timestamp int64) {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Failed to create directory", "directory", dir, "error", err)
		return
	}

	data := map[string]interface{}{
		"timestamp":  timestamp,
		"bidRequest": bidRequest,
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file", "filename", filename, "error", err)
		return
	}
	defer file.Close()

	// Read existing data
	var existingData []map[string]interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&existingData); err != nil && err.Error() != "EOF" {
		log.Error("Failed to decode existing JSON data", "error", err)
		return
	}

	// Append new data
	existingData = append(existingData, data)

	// Write the updated data back to the file
	file.Seek(0, 0)
	file.Truncate(0)
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(existingData); err != nil {
		log.Error("Failed to encode data to JSON", "error", err)
	}
}

// saveBidResponses saves bid responses to a JSON file
func saveBidResponses(filename string, responses []interface{}) {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error("Failed to create directory", "directory", dir, "error", err)
		return
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open file", "filename", filename, "error", err)
		return
	}
	defer file.Close()

	// Read existing data
	var existingData []interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&existingData); err != nil && err.Error() != "EOF" {
		log.Error("Failed to decode existing JSON data", "error", err)
		return
	}

	// Append new responses
	existingData = append(existingData, responses...)

	// Write the updated responses back to the file
	file.Seek(0, 0)
	file.Truncate(0)
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(existingData); err != nil {
		log.Error("Failed to encode data to JSON", "error", err)
	}
}
