package mevcommit

import (
	"log"
	"os"

	pb "github.com/primev/mev-commit/p2p/gen/go/bidderapi/v1"
	"google.golang.org/grpc"

	"fmt"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds the configuration settings.
type Config struct {
	ServerAddress string `json:"server_address" yaml:"server_address"`
	LogFmt        string `json:"log_fmt" yaml:"log_fmt"`
	LogLevel      string `json:"log_level" yaml:"log_level"`
}

// NewBidClient creates a new gRPC client connection to the bidder service and returns a bidder instance.
func NewBidClient(cfg Config) (*bidder, error) {
	conn, err := grpc.NewClient(cfg.ServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to gRPC server: %v", err)
		return nil, err
	}

	client := pb.NewBidderClient(conn)
	return &bidder{client: client}, nil
}

// connect to mev-commit chain
func NewMevCommitClient(endpoint string) (*ethclient.Client, error) {
	client, err := rpc.Dial(endpoint)
	if err != nil {
		log.Println(err)
	}

	ec := ethclient.NewClient(client)
	return ec, nil
}

// LoadABI loads the ABI from the specified file path
func LoadABI(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
