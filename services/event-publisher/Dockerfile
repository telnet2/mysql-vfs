# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the event-publisher binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o event-publisher ./services/event-publisher

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/event-publisher .

# Expose HTTP port
EXPOSE 8083

# Run the event-publisher
CMD ["./event-publisher"]
