// Package mevcommit provides functionality for interacting with the mev-commit protocol,
// including setting up a bidder client, connecting to an Ethereum node, and handling
// account authentication.
package mevcommit

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"

	pb "github.com/primev/preconf_blob_bidder/core/bidderpb"
	"google.golang.org/grpc"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// BidderConfig holds the configuration settings for the mev-commit bidder node.
type BidderConfig struct {
	ServerAddress string `json:"server_address" yaml:"server_address"` // The address of the gRPC server for the bidder node.
	LogFmt        string `json:"log_fmt" yaml:"log_fmt"`               // The format for logging output.
	LogLevel      string `json:"log_level" yaml:"log_level"`           // The level of logging detail.
}

// Bidder utilizes the mev-commit bidder client to interact with the mev-commit chain.
type Bidder struct {
	client pb.BidderClient // gRPC client for interacting with the mev-commit bidder service.
}

// GethConfig holds configuration settings for a Geth node to connect to the mev-commit chain.
type GethConfig struct {
	Endpoint string `json:"endpoint" yaml:"endpoint"` // The RPC endpoint for connecting to the Ethereum node.
}

// AuthAcct holds the private key, public key, address, and transaction authorization information for an account.
type AuthAcct struct {
	PrivateKey *ecdsa.PrivateKey  // The private key for the account.
	PublicKey  *ecdsa.PublicKey   // The public key derived from the private key.
	Address    common.Address     // The Ethereum address derived from the public key.
	Auth       *bind.TransactOpts // The transaction options for signing transactions.
}

// NewBidderClient creates a new gRPC client connection to the bidder service and returns a Bidder instance.
//
// Parameters:
// - cfg: The BidderConfig struct containing the server address and logging settings.
//
// Returns:
// - A pointer to a Bidder struct, or an error if the connection fails.
func NewBidderClient(cfg BidderConfig) (*Bidder, error) {
	// Establish a gRPC connection to the bidder service
	conn, err := grpc.Dial(cfg.ServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to gRPC server: %v", err)
		return nil, err
	}

	// Create a new bidder client using the gRPC connection
	client := pb.NewBidderClient(conn)
	return &Bidder{client: client}, nil
}

// NewGethClient connects to an Ethereum-compatible chain using the provided RPC endpoint.
//
// Parameters:
// - endpoint: The RPC endpoint of the Ethereum node.
//
// Returns:
// - A pointer to an ethclient.Client for interacting with the Ethereum node, or an error if the connection fails.
func NewGethClient(endpoint string) (*ethclient.Client, error) {
	// Dial the Ethereum RPC endpoint
	client, err := rpc.Dial(endpoint)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// Create a new ethclient.Client using the RPC client
	ec := ethclient.NewClient(client)
	log.Println("geth client connected")
	return ec, nil
}

// AuthenticateAddress converts a hex-encoded private key string to an AuthAcct struct,
// which contains the account's private key, public key, address, and transaction authorization.
//
// Parameters:
// - privateKeyHex: The hex-encoded private key string.
//
// Returns:
// - A pointer to an AuthAcct struct, or an error if authentication fails.
func AuthenticateAddress(privateKeyHex string) (*AuthAcct, error) {
	if privateKeyHex == "" {
		return nil, nil
	}

	// Convert the hex-encoded private key to an ECDSA private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to load private key: %v", err)
		return nil, err
	}

	// Extract the public key from the private key
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("Failed to assert public key type")
	}

	// Generate the Ethereum address from the public key
	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	// Set the chain ID (currently hardcoded for Holesky testnet)
	chainID := big.NewInt(17000) // Holesky

	// Create the transaction options with the private key and chain ID
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		log.Fatalf("Failed to create authorized transactor: %v", err)
	}

	// Return the AuthAcct struct containing the private key, public key, address, and transaction options
	return &AuthAcct{
		PrivateKey: privateKey,
		PublicKey:  publicKeyECDSA,
		Address:    address,
		Auth:       auth,
	}, nil
}
