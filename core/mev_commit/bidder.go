package biddingwindow

import (
	"context"
	"fmt"

	pb "github.com/primev/mev-commit/p2p/gen/go/bidderapi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type Config struct {
	ServerAddress string `json:"server_address" yaml:"server_address"`
	LogFmt        string `json:"log_fmt" yaml:"log_fmt"`
	LogLevel      string `json:"log_level" yaml:"log_level"`
}

type BiddingWindow struct {
	client pb.BidderClient
}

func NewBiddingWindow(grpcAddress string) (*BiddingWindow, error) {
	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	client := pb.NewBidderClient(conn)
	return &BiddingWindow{client: client}, nil
}

// GetMinDeposit retrieves the minimum deposit required for bidding from the server.
func GetMinDeposit(cfg Config) (*pb.DepositResponse, error) {
	creds := insecure.NewCredentials()
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewBidderClient(conn)
	ctx := context.Background()
	response, err := client.GetMinDeposit(ctx, &pb.EmptyMessage{})
	if err != nil {
		return nil, err
	}

	return response, nil
}

// DepositMinBidAmount deposits the minimum bid amount into the bidding window.
func DepositMinBidAmount(cfg Config) (int64, error) {
	minDepositResponse, err := GetMinDeposit(cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to get minimum deposit: %w", err)
	}

	minDepositAmount := minDepositResponse.Amount
	creds := insecure.NewCredentials()
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	client := pb.NewBidderClient(conn)
	depositRequest := &pb.DepositRequest{
		Amount: minDepositAmount,
	}

	ctx := context.Background()
	response, err := client.Deposit(ctx, depositRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to deposit funds: %w", err)
	}

	windowNumber := int64(response.WindowNumber.Value)
	fmt.Printf("Deposited minimum bid amount successfully into window number: %v\n", windowNumber)
	return windowNumber, nil
}

// WithdrawFunds withdraws the deposited funds from the specified bidding window.
func WithdrawFunds(cfg Config, windowNumber int64) error {
	creds := insecure.NewCredentials()
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewBidderClient(conn)
	withdrawRequest := &pb.WithdrawRequest{
		WindowNumber: wrapperspb.UInt64(uint64(windowNumber)),
	}

	ctx := context.Background()
	response, err := client.Withdraw(ctx, withdrawRequest)
	if err != nil {
		return fmt.Errorf("failed to withdraw funds: %w", err)
	}

	fmt.Printf("Withdraw successful: %v\n", response)
	return nil
}
