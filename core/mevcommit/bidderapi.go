// Package mevcommit provides functionality for interacting with the mev-commit protocol,
// including sending bids for blob transactions and saving bid requests and responses.
package mevcommit

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	pb "github.com/primev/preconf_blob_bidder/core/bidderpb"
)

// Define the BidRequest struct to match the API
type BidRequest struct {
    TxHashes            []string `json:"txHashes,omitempty"`
    RawTransactions     []string `json:"rawTransactions,omitempty"`
    Amount              string   `json:"amount"`
    BlockNumber         int64    `json:"blockNumber"`
    DecayStartTimestamp int64    `json:"decayStartTimestamp"`
    DecayEndTimestamp   int64    `json:"decayEndTimestamp"`
    RevertingTxHashes   []string `json:"revertingTxHashes,omitempty"`
}

// Define the BidResponse struct to match the expected response
type BidResponse struct {
    Result struct {
        TxHashes             []string `json:"txHashes"`
        BidAmount            string   `json:"bidAmount"`
        BlockNumber          string   `json:"blockNumber"`
        ReceivedBidDigest    string   `json:"receivedBidDigest"`
        ReceivedBidSignature string   `json:"receivedBidSignature"`
        CommitmentDigest     string   `json:"commitmentDigest"`
        CommitmentSignature  string   `json:"commitmentSignature"`
        ProviderAddress      string   `json:"providerAddress"`
        DecayStartTimestamp  string   `json:"decayStartTimestamp"`
        DecayEndTimestamp    string   `json:"decayEndTimestamp"`
        DispatchTimestamp    string   `json:"dispatchTimestamp"`
        RevertingTxHashes    []string `json:"revertingTxHashes"`
    } `json:"result"`
}

// Update the SendBid function
func (b *Bidder) SendBid(input interface{}, amount string, blockNumber, decayStart, decayEnd int64) (*BidResponse, error) {
    var txHashes []string
    var rawTransactions []string

    // Determine the input type and process accordingly
    switch v := input.(type) {
    case []string:
        txHashes = make([]string, len(v))
        for i, hash := range v {
            txHashes[i] = strings.TrimPrefix(hash, "0x")
        }
    case []*types.Transaction:
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

    // Create a new bid request
    bidRequest := &BidRequest{
        Amount:              amount,
        BlockNumber:         blockNumber,
        DecayStartTimestamp: decayStart,
        DecayEndTimestamp:   decayEnd,
        RevertingTxHashes:   []string{},
    }

    if len(txHashes) > 0 {
        bidRequest.TxHashes = txHashes
    } else if len(rawTransactions) > 0 {
        bidRequest.RawTransactions = rawTransactions
    }

    log.Info(fmt.Sprintf("Bid request details:\n"+
        "txHashes: %v\n"+
        "rawTransactions: %v\n"+
        "preconf_bid_amt: %s\n"+
        "blockNumber: %d\n"+
        "decayStart: %d\n"+
        "decayEnd: %d",
        bidRequest.TxHashes,
        bidRequest.RawTransactions,
        bidRequest.Amount,
        bidRequest.BlockNumber,
        bidRequest.DecayStartTimestamp,
        bidRequest.DecayEndTimestamp,
    ))

    // Marshal the bidRequest to JSON
    jsonData, err := json.Marshal(bidRequest)
    if err != nil {
        log.Error("Failed to marshal bid request to JSON", "error", err)
        return nil, fmt.Errorf("failed to marshal bid request to JSON: %w", err)
    }

    // Create an HTTP client with a timeout
    client := &http.Client{
        Timeout: 15 * time.Second,
    }

    // Build the request URL
    url := fmt.Sprintf("http://%s/v1/bidder/bid", "127.0.0.1:13523")

    // Create an HTTP POST request
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        log.Error("Failed to create HTTP request", "error", err)
        return nil, fmt.Errorf("failed to create HTTP request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")

    // Send the HTTP request
    resp, err := client.Do(req)
    if err != nil {
        log.Error("Failed to send HTTP request", "error", err)
        return nil, fmt.Errorf("failed to send HTTP request: %w", err)
    }
    defer resp.Body.Close()

    // Read the response body
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        log.Error("Failed to read response body", "error", err)
        return nil, fmt.Errorf("failed to read response body: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        log.Error("Received non-OK response", "status", resp.StatusCode, "body", string(body))
        return nil, fmt.Errorf("received non-OK response: %d, body: %s", resp.StatusCode, string(body))
    }

    // Parse the response body into BidResponse
    var bidResponse BidResponse
    err = json.Unmarshal(body, &bidResponse)
    if err != nil {
        log.Error("Failed to unmarshal response body", "error", err)
        return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
    }

    log.Info("Received bid response", "response", bidResponse)

    return &bidResponse, nil
}







// OLD w/ GRPC 10/11/24
// SendBid sends a bid to the mev-commit client for a given set of transaction hashes or raw transactions, amount, and block number.
// The bid will be decayed over a specified time range.

// Parameters:
// - input: Can be either a slice of transaction hashes ([]string) or a slice of *types.Transaction.
// - amount: The bid amount in wei as a string.
// - blockNumber: The block number for which the bid applies.
// - decayStart: The start timestamp for bid decay (in milliseconds).
// - decayEnd: The end timestamp for bid decay (in milliseconds).

// Returns:
// - A pb.Bidder_SendBidClient to receive bid responses, or an error if the bid fails.
// func (b *Bidder) SendBid(input interface{}, amount string, blockNumber, decayStart, decayEnd int64) (pb.Bidder_SendBidClient, error) {
//     var txHashes []string
//     var rawTransactions []string

//     // Determine the input type and process accordingly
//     switch v := input.(type) {
//     case []string:
//         txHashes = make([]string, len(v))
//         for i, hash := range v {
//             txHashes[i] = strings.TrimPrefix(hash, "0x")
//         }
//     case []*types.Transaction:
//         rawTransactions = make([]string, len(v))
//         for i, tx := range v {
//             rlpEncodedTx, err := tx.MarshalBinary()
//             if err != nil {
//                 log.Error("Failed to marshal transaction to raw format", "error", err)
//                 return nil, fmt.Errorf("failed to marshal transaction: %w", err)
//             }
//             rawTransactions[i] = hex.EncodeToString(rlpEncodedTx) // Don't convert to string
//         }
//     default:
//         log.Warn("Unsupported input type, must be []string or []*types.Transaction")
//         return nil, fmt.Errorf("unsupported input type: %T", input)
//     }

//     // Create a new bid request
//     bidRequest := &pb.Bid{
//         Amount:              amount,
//         BlockNumber:         blockNumber,
//         DecayStartTimestamp: decayStart,
//         DecayEndTimestamp:   decayEnd,
//     }

//     if len(txHashes) > 0 {
//         bidRequest.TxHashes = txHashes
//     } else if len(rawTransactions) > 0 {
//         bidRequest.RawTransactions = rawTransactions
//     }

// 	log.Info(fmt.Sprintf("Bid request details:\n"+
//     "txHashes: %v\n"+
//     "rawTransactions: %v\n"+
//     "preconf_bid_amt: %s\n"+
//     "blockNumber: %d\n"+
//     "decayStart: %d\n"+
//     "decayEnd: %d",
//     bidRequest.TxHashes,
//     bidRequest.RawTransactions,
//     bidRequest.Amount,
//     bidRequest.BlockNumber,
//     bidRequest.DecayStartTimestamp,
//     bidRequest.DecayEndTimestamp,
// ))

//     // Create a context with a timeout
//     ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
//     defer cancel()

//     // Send the bid request to the mev-commit client
//     response, err := b.client.SendBid(ctx, bidRequest)
//     if err != nil {
//         log.Error("Failed to send bid", "error", err)
//         return nil, fmt.Errorf("failed to send bid: %w", err)
//     }

//     return response, nil
// }




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
