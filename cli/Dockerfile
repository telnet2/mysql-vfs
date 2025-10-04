FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build both CLI versions
WORKDIR /app/cli
RUN CGO_ENABLED=0 GOOS=linux go build -o vfs-cli main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o vfs-cli-prompt main_prompt.go

FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache ca-certificates jq curl bash zsh

COPY --from=builder /app/cli/vfs-cli .
COPY --from=builder /app/cli/vfs-cli-prompt .

# Default to prompt version
CMD ["./vfs-cli-prompt"]
