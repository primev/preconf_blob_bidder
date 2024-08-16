## About
This repository provides an example workflow that attaches preconfirmations to blob transactions. The script supports sending transactions to a public endpoint or directly to the Titan relay for private transactions. This project demonstrates how to send blob transactions with preconfirmation bids in the Ethereum Holesky testnet. It includes a script to interact with a mev-commit p2p bidder node.


## Requirements
- funded holesky address
- funded mev-commit address
- mev-commit p2p bidder node v0.5.0 or higher

## Installation
```git clone https://github.com/your-repository/preconf_blob_bidder.git
cd preconf_blob_bidder
```

Then `go mod tiny` to install dependencies.

## Making a preconf bid
1. Ensure the mev-commit bidder node is starting in the background. See [here](https://docs.primev.xyz/get-started/quickstart) for a quickstart. This is the following command to use: 
`curl -L -o launchmevcommit launch.mev-commit.xyz; chmod +x launchmevcommit; ./launchmevcommit --node-type bidder`

2. `go run cmd/preconfethtransfer.go --endpoint endpoint --privatekey private_key` where `endpoint` is the endpoint of the Holesky node and `private_key` is the private key of the account that will be used to send the transactions.
* --endpoint: The RPC endpoint of your Ethereum Holesky node.
* --privatekey: The private key of the account that will send the transaction.
* --private: Set this flag to True to send the transaction privately to the Titan relay. Otherwise, it will send the transaction to the public Ethereum endpoint.

### `sendblob.go()`
Main Logic:

The main() function sets up the mev-commit bidder client and connects to the Ethereum client using the provided endpoint.
It checks for pending transactions in a loop, sending a new blob transaction if no transactions are pending.
The loop runs for 12 hours, after which it stops.
sendPreconfBid:

`sendPreconfBid` sends a preconfirmation bid for a transaction. The bid amount and decay period are hardcoded but can be adjusted if needed.
checkPendingTxs:

`checkPendingTxs` checks the status of transactions that were sent. If a transaction is still pending, it resends a preconfirmation bid. If the transaction is confirmed, it removes it from the pending transactions list.