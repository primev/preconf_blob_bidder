package mevcommit

import (
	pb "github.com/primev/mev-commit/p2p/gen/go/bidderapi/v1"
	"google.golang.org/grpc"

	"fmt"

	"google.golang.org/grpc/credentials/insecure"
)

// Config holds the configuration settings.
type Config struct {
	ServerAddress string `json:"server_address" yaml:"server_address"`
	LogFmt        string `json:"log_fmt" yaml:"log_fmt"`
	LogLevel      string `json:"log_level" yaml:"log_level"`
}

// newBidderClient creates a new gRPC client connection to the bidder service
func newBidderClient(cfg Config) (pb.BidderClient, error) {
	conn, err := grpc.NewClient(cfg.ServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	client := pb.NewBidderClient(conn)
	return client, nil
}
