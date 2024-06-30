package main

import (
	"context"

	"flag"
	"log"

	bb "github.com/primev/preconf_blob_bidder/core/mevcommit"
)

// run with go run cmd/bidwindowmevcommit.go --privatekey "private key" --endpoint "endpoint"
// This script mimics the same bidder functionality in the mev-commit bidder API, but calling the smart contracts
// directly using Geth. The minimum bid amount is retrieved from the blockTracker contract and used as the default
// deposit amount. Once the amount is deposited, the script calls `getDeposit` to confirm the deposit.

// The script needs to wait about 12 minutes before the funds are available to be withdrawn. This is an overestimation. Not sure why.
// Each window is 10 blocks, so about 120 seconds. After 360 seconds, or 6 minutes it should be good to withdraw from the window.
// The oracle lag also needs to be taken into account, which lags behind by 20 blocks.

func main() {
	// Define command-line flags for the private key and endpoint
	// privateKeyHex := flag.String("privatekey", "", "The private key in hex format")
	endpoint := flag.String("endpoint", "", "The Ethereum client endpoint")
	flag.Parse()
	if *endpoint == "" {
		log.Fatal("Endpoint is required. Use the -endpoint flag to provide it.")
	}

	// NewGethClient connects to the MEV-Commit chain given an endpoint.
	client, err := bb.NewGethClient(*endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to MEV-Commit chain: %v", err)
	}

	// Get block number for mev-commit
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("mev-commit Block Number: ", blockNumber)

	// WindowHeight
	currentWindow, err := bb.WindowHeight("abi/BlockTracker.abi", client)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("Current Bidding Window: ", currentWindow)

	// // load bidderRegistry contract
	// bidderRegistryABI, err := bb.LoadABI("abi/BidderRegistry.abi")
	// if err != nil {
	// 	log.Println("Failed to load ABI file:", err)
	// 	return
	// }

	// bidderRegistryContract := bind.NewBoundContract(common.HexToAddress(bidderRegistryAddress), bidderRegistryABI, client, client, client)

	// // Authenticate address with AuthenticateAddress
	// authAcct, err := bb.AuthenticateAddress(*privateKeyHex, client)
	// if err != nil {
	// 	log.Fatalf("Failed to connect to MEV-Commit chain: %v", err)
	// }

	// // PART 2: DEPOSIT INTO BIDDING CONTRACT
	// // Call the minDeposit function
	// var minDepositResult []interface{}
	// err = bidderRegistryContract.Call(nil, &minDepositResult, "minDeposit")
	// if err != nil {
	// 	log.Println("Failed to call minDeposit function: ", err)
	// 	return
	// }

	// // Extract the minDeposit as *big.Int
	// minDeposit, ok := minDepositResult[0].(*big.Int)
	// if !ok {
	// 	log.Println("Failed to convert minDeposit to *big.Int")
	// 	return
	// }

	// log.Println("Min Deposit: ", minDeposit)

	// // Set the value to minDeposit
	// authAcct.Auth.Value = minDeposit

	// // Prepare the transaction
	// tx, err := bidderRegistryContract.Transact(authAcct.Auth, "depositForSpecificWindow", currentWindow)
	// if err != nil {
	// 	log.Fatalf("Failed to create transaction: %v", err)
	// }

	// fmt.Printf("Transaction sent: %s\n", tx.Hash().Hex())

	// // Wait for the transaction to be mined (optional)
	// receipt, err := bind.WaitMined(context.Background(), client, tx)
	// if err != nil {
	// 	log.Fatalf("Transaction mining error: %v", err)
	// }

	// if receipt.Status == 1 {
	// 	fmt.Println("Transaction successful")
	// } else {
	// 	fmt.Println("Transaction failed")
	// }

	// // PART 2.5: Confirm bidder deposit
	// // getDeposit(address bidder,uint256 window)
	// var depositResult []interface{}
	// err = bidderRegistryContract.Call(nil, &depositResult, "getDeposit", authAcct.Address, currentWindow)
	// if err != nil {
	// 	log.Fatalf("Failed to call getDeposit function: %v", err)
	// }

	// // Extract the deposit amount as *big.Int
	// depositAmount, ok := depositResult[0].(*big.Int)
	// if !ok {
	// 	log.Fatalf("Failed to convert deposit amount to *big.Int")
	// }

	// fmt.Printf("Deposit Amount: %s\n", depositAmount.String())

	// // Wait for 11 minutes before withdrawing. This is an overestimated time to ensure that the next window has started.
	// log.Println("Waiting for 11 minutes before withdrawing...")
	// time.Sleep(11 * time.Minute)

	// // PART 3: WITHDRAW FUNDS
	// // withdraw funds
	// // withdrawBidderAmountFromWindow(address payable bidder,uint256 window)
	// withdrawAmount := big.NewInt(183269)
	// withdrawalTx, err := bidderRegistryContract.Transact(authAcct.Auth, "withdrawBidderAmountFromWindow", authAcct.Address, withdrawAmount)
	// if err != nil {
	// 	log.Fatalf("Failed to create withdrawal transaction: %v", err)
	// }

	// fmt.Printf("Withdrawal Transaction sent: %s\n", withdrawalTx.Hash().Hex())

	// // Wait for the withdrawal transaction to be mined
	// withdrawalReceipt, err := bind.WaitMined(context.Background(), client, withdrawalTx)
	// if err != nil {
	// 	log.Fatalf("Withdrawal transaction mining error: %v", err)
	// }

	// if withdrawalReceipt.Status == 1 {
	// 	fmt.Println("Withdrawal successful")
	// } else {
	// 	fmt.Println("Withdrawal failed")
	// }
}
