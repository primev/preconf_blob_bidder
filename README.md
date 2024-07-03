
### About
This repository shows an example workflow that attaches preconfirmations to different kinds of transactions.

### Requirements
- funded holesky address
- funded mev-commit address
- mev-commit p2p bidder node 

### Installation
```git clone https://github.com/your-repository/preconf_blob_bidder.git
cd preconf_blob_bidder```


### Making a preconf bid
1. Ensure the mev-commit bidder node is starting in the background. See [here](https://docs.primev.xyz/get-started/quickstart) for a quickstart. If the mev-commit binary is already downloaded, can simply run `./launchmevcommit --node-type bidder` in the directory where the binary is located.
2. `go run cmd/preconfethtransfer.go --endpoint endpoint --privatekey private_key` where `endpoint` is the endpoint of the Holesky node and `private_key` is the private key of the account that will be used to send the transactions.



