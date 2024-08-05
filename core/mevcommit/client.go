package mevcommit

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"

	pb "github.com/primev/preconf_blob_bidder/core/bidderpb"
	"google.golang.org/grpc"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// BidderConfig holds the configuration settings for mev-commit bidder node.
type BidderConfig struct {
	ServerAddress string `json:"server_address" yaml:"server_address"`
	LogFmt        string `json:"log_fmt" yaml:"log_fmt"`
	LogLevel      string `json:"log_level" yaml:"log_level"`
}

// Bidder utilizes the mevcommit bidder client to interact with the mevcommit chain.
type Bidder struct {
	client pb.BidderClient
}

// GethConfig holds configuration settings for a geth node to connect to mev-commit chain.
type GethConfig struct {
	Endpoint string `json:"endpoint" yaml:"endpoint"`
}

// AuthAcct holds the private key, public key, address, and authentication for a given private key.
type AuthAcct struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
	Address    common.Address
	Auth       *bind.TransactOpts
}

// NewBidderClient creates a new gRPC client connection to the bidder service and returns a bidder instance.
func NewBidderClient(cfg BidderConfig) (*Bidder, error) {
	conn, err := grpc.NewClient(cfg.ServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Failed to connect to gRPC server: %v", err)
		return nil, err
	}

	client := pb.NewBidderClient(conn)
	return &Bidder{client: client}, nil
}

// NewGethClient connects to the MEV-Commit chain given an endpoint.
func NewGethClient(endpoint string) (*ethclient.Client, error) {
	client, err := rpc.Dial(endpoint)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	ec := ethclient.NewClient(client)
	return ec, nil
}

// AuthenticateAddress converts a hex-encoded private key string to a AuthAcct struct.
func AuthenticateAddress(privateKeyHex string, client *ethclient.Client) (*AuthAcct, error) {
	if privateKeyHex == "" {
		return nil, nil
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Printf("Failed to load private key: %v", err)
		return nil, err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("Failed to assert public key type")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		log.Fatalf("Failed to create authorized transactor: %v", err)
	}

	return &AuthAcct{
		PrivateKey: privateKey,
		PublicKey:  publicKeyECDSA,
		Address:    address,
		// Nonce:      nonce,
		Auth: auth,
	}, nil
}
