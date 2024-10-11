# Use the official Go image as the builder stage
FROM golang:1.23 as builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project and build the Go application
COPY . .
RUN go build -o sendblob ./cmd/sendblob.go

# Use a minimal base image for the final stage
FROM debian:bookworm

# Install CA certificates and any other necessary dependencies
RUN apt-get update && \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*
    
# Set the working directory inside the container
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/sendblob .

# Command to run when starting the container
CMD ["./sendblob"]
