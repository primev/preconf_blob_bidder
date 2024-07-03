package mevcommit

import (
	"context"
	"fmt"

	pb "github.com/primev/mev-commit/p2p/gen/go/bidderapi/v1"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

// WithdrawFunds withdraws the deposited funds from the specified bidding window.
func (b *Bidder) WithdrawFunds(windowNumber int64) error {
	withdrawRequest := &pb.WithdrawRequest{
		WindowNumber: wrapperspb.UInt64(uint64(windowNumber)),
	}

	ctx := context.Background()
	response, err := b.client.Withdraw(ctx, withdrawRequest)
	if err != nil {
		return fmt.Errorf("failed to withdraw funds: %w", err)
	}

	fmt.Printf("Withdraw successful: %v\n", response)
	return nil
}

// SendBid sends a preconf bid with the specified parameters.
func (b *Bidder) SendBid(txHashes []string, amount string, blockNumber, decayStart, decayEnd int64) error {
	bidRequest := &pb.Bid{
		TxHashes:            txHashes,
		Amount:              amount,
		BlockNumber:         blockNumber,
		DecayStartTimestamp: decayStart,
		DecayEndTimestamp:   decayEnd,
	}

	ctx := context.Background()
	response, err := b.client.SendBid(ctx, bidRequest)
	if err != nil {
		return fmt.Errorf("failed to send bid: %w", err)
	}

	fmt.Printf("Bid sent successfully: %v\n", response)
	return nil
}
