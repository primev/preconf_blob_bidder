package eth

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type FlashbotsPayload struct {
	Jsonrpc string                   `json:"jsonrpc"`
	Method  string                   `json:"method"`
	Params  []map[string]interface{} `json:"params"`
	ID      int                      `json:"id"`
}

func sendBundle(RPCURL string, signedTx *types.Transaction, blkNum uint64) (string, error) {
	binary, err := signedTx.MarshalBinary()
	if err != nil {
		log.Fatalf("Error marshal transaction: %v", err)
	}

	blockNum := hexutil.EncodeUint64(blkNum)

	payload := FlashbotsPayload{
		Jsonrpc: "2.0",
		Method:  "eth_sendBundle",
		Params: []map[string]interface{}{
			{
				"txs": []string{
					hexutil.Encode(binary),
				},
				"blockNumber": blockNum,
			},
		},
		ID: 1,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	httpClient := &http.Client{}
	req, err := http.NewRequest("POST", RPCURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	return string(body), nil
}
