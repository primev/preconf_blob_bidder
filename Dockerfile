FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o bidder ./cmd/sendblob.go

FROM alpine:latest


RUN apk --no-cache add curl
RUN apk add --no-cache jq

COPY --from=builder /app/bidder /app/bidder
COPY --from=builder /app/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
