go run cmd/sendblob.go --rpc-endpoints "http://186.233.187.165:8545/" --ws-endpoint ws://186.233.187.165:8546 --privatekey 45f110daadc69f88b068dec7cdce73ca60c14fe853b2e3ef3ea0d0731171a8e5 -use-payload=false (or use-payload=true)

./launchmevcommit --node-type bidder

https://holesky.etherscan.io/address/0xe51ef1836dbef052bffd2eb3fe1314365d23129d

curl -L -o launchmevcommit launch.mev-commit.xyz; chmod +x launchmevcommit; ./launchmevcommit --node-type bidder


### connect to single preconf builder 2 endpoint
go run cmd/sendblob.go --rpc-endpoints "http://52.11.201.67:8545" --ws-endpoint wss://ethereum-holesky.core.chainstack.com/41ec2970d37bf8ded74615a987692adf --privatekey 45f110daadc69f88b068dec7cdce73ca60c14fe853b2e3ef3ea0d0731171a8e5 -use-payload=true


### connect to multiple private builder endpoints:
go run cmd/sendblob.go --rpc-endpoints "http://52.11.201.67:8545,http://186.233.187.165:8545/" \
--ws-endpoint wss://ethereum-holesky.core.chainstack.com/41ec2970d37bf8ded74615a987692adf \
--privatekey 45f110daadc69f88b068dec7cdce73ca60c14fe853b2e3ef3ea0d0731171a8e5 \
-use-payload=true


### connect to public endpoint...
go run cmd/sendblob.go --rpc-endpoints "https://ethereum-holesky.core.chainstack.com/41ec2970d37bf8ded74615a987692adf" \
--ws-endpoint wss://ethereum-holesky.core.chainstack.com/41ec2970d37bf8ded74615a987692adf \
--privatekey 45f110daadc69f88b068dec7cdce73ca60c14fe853b2e3ef3ea0d0731171a8e5 \
-use-payload=true


### Docker
super important command to check logs `sudo docker-compose logs bidder-node`


export RPC_ENDPOINTS="https://ethereum-holesky.core.chainstack.com/41ec2970d37bf8ded74615a987692adf"
export WS_ENDPOINT="wss://ethereum-holesky.core.chainstack.com/41ec2970d37bf8ded74615a987692adf"
export PRIVATE_KEY="45f110daadc69f88b068dec7cdce73ca60c14fe853b2e3ef3ea0d0731171a8e5"
export USE_PAYLOAD=true
export BIDDER_ADDRESS="127.0.0.1:13523"

go run cmd/sendblob.go