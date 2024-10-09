#!/bin/sh

/app/bidder				\
    --rpc-endpoints ${RPC_ENDPOINTS} 	\
    --ws-endpoint ${WS_ENDPOINT}	\
    --privatekey ${PRIVATE_KEY}		\
    --use-payload ${USE_PAYLOAD}

