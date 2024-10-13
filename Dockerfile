FROM golang:1.21

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project into the container
COPY . .

# Build the Go application from the 'eth_transfer' directory
RUN go build -o sendblob ./cmd

# Command to run your application
CMD ["./sendblob"]
