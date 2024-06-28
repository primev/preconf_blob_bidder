package mevcommit

import (
	"testing"
)

func TestNewBidderClient(t *testing.T) {
	cfg := Config{
		ServerAddress: "localhost:13524", // Default address for mevcommit gRPC server
		LogFmt:        "json",            // Example log format
		LogLevel:      "info",            // Example log level
	}

	client, err := newBidderClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client == nil {
		t.Errorf("Expected non-nil client")
	}
}