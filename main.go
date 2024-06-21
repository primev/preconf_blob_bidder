package main

import (
	"context"
	"fmt"
	"time"

	pb "github.com/primev/mev-commit/p2p/gen/go/bidderapi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type config struct {
	ServerAddress string `json:"server_address" yaml:"server_address"`
	LogFmt        string `json:"log_fmt" yaml:"log_fmt"`
	LogLevel      string `json:"log_level" yaml:"log_level"`
}

func main() {
	cfg := config{
		ServerAddress: "localhost:13524",
		LogFmt:        "text",
		LogLevel:      "info",
	}
	// print the start time
	fmt.Println("Start time: ", time.Now())
	// DEPOSIT
	response, err := getMinDeposit(cfg)
	if err != nil {
		fmt.Printf("failed to get minimum deposit: %v\n", err)
	}

	fmt.Printf("Minimum deposit required: %v\n", response)

	// TODO - At time n, increment window number by n+2 so that funds are available to make bids in window n+1
	// Get minimum amount to deposit and log the window number
	windowNumber, err := depositMinBidAmount(cfg)
	if err != nil {
		fmt.Printf("failed to deposit minimum bid amount: %v\n", err)
		return
	}

	fmt.Printf("Deposited into window number: %v\n", windowNumber)

	// Wait for 11 minutes before withdrawing the funds
	// Need to wait n-2 windows for settlement, so that is about 30 blocks = 360 seconds, or 6 minutes.
	// Oracle lags behind by 20 blocks, so an additional 4 minutes.
	// Total wait time comes out to ~10 minutes at the earliest. 
	fmt.Println("Waiting for 11 minutes before withdrawing the funds...")
	time.Sleep(11 * time.Minute)

	// WITHDRAW from window
	if err := withdrawFunds(cfg, windowNumber); err != nil {
		fmt.Printf("failed to withdraw funds: %v\n", err)
	}

	fmt.Println("End time: ", time.Now())

}

// getMinDeposit retrieves the minimum deposit required for bidding from the server.
//
// Parameters:
// - cfg: Configuration containing server address.
//
// Returns:
// - *pb.DepositResponse: The response containing the minimum deposit amount.
// - error: Any error encountered during the process.
func getMinDeposit(cfg config) (*pb.DepositResponse, error) {
	// Set up insecure credentials for gRPC connection.
	creds := insecure.NewCredentials()
	// Establish a connection to the server.
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Create a new BidderClient.
	client := pb.NewBidderClient(conn)

	// Call GetMinDeposit to retrieve the minimum deposit amount.
	ctx := context.Background()
	response, err := client.GetMinDeposit(ctx, &pb.EmptyMessage{})
	if err != nil {
		return nil, err
	}

	return response, nil
}

// depositMinBidAmount deposits the minimum bid amount into the bidding window.
//
// Parameters:
// - cfg: Configuration containing server address.
//
// Returns:
// - int64: The window number where the deposit was made.
// - error: Any error encountered during the process.
func depositMinBidAmount(cfg config) (int64, error) {
	// Get the minimum deposit amount.
	minDepositResponse, err := getMinDeposit(cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to get minimum deposit: %w", err)
	}

	// Extract the minimum deposit amount from the response.
	minDepositAmount := minDepositResponse.Amount

	// Set up insecure credentials for gRPC connection.
	creds := insecure.NewCredentials()
	// Establish a connection to the server.
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	// Create a new BidderClient.
	client := pb.NewBidderClient(conn)

	// Create a DepositRequest with the minimum deposit amount.
	depositRequest := &pb.DepositRequest{
		Amount: minDepositAmount,
	}

	// Call Deposit to deposit the minimum bid amount.
	ctx := context.Background()
	response, err := client.Deposit(ctx, depositRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to deposit funds: %w", err)
	}

	// Convert the window number to int64.
	windowNumber := int64(response.WindowNumber.Value)
	fmt.Printf("Deposited minimum bid amount successfully into window number: %v\n", windowNumber)
	return windowNumber, nil
}

// withdrawFunds withdraws the deposited funds from the specified bidding window.
//
// Parameters:
// - cfg: Configuration containing server address.
// - windowNumber: The window number from which to withdraw funds.
//
// Returns:
// - error: Any error encountered during the process.
func withdrawFunds(cfg config, windowNumber int64) error {
	// Set up insecure credentials for gRPC connection.
	creds := insecure.NewCredentials()
	// Establish a connection to the server.
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	defer conn.Close()

	// Create a new BidderClient.
	client := pb.NewBidderClient(conn)

	// Create a WithdrawRequest with the specified window number.
	withdrawRequest := &pb.WithdrawRequest{
		WindowNumber: wrapperspb.UInt64(uint64(windowNumber)),
	}

	// Call Withdraw to withdraw the funds.
	ctx := context.Background()
	response, err := client.Withdraw(ctx, withdrawRequest)
	if err != nil {
		return fmt.Errorf("failed to withdraw funds: %w", err)
	}

	fmt.Printf("Withdraw successful: %v\n", response)
	return nil
}
