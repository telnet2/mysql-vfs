# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make ca-certificates

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies with git credentials
RUN --mount=type=secret,id=git-credentials,target=/root/.git-credentials \
    --mount=type=cache,target=/go/pkg/mod \
    go env -w GOPRIVATE=code.byted.org && \
    git config --global credential.helper store && \
    go mod download

# Copy source code
COPY . .

# Build both CLI versions
WORKDIR /app/cli
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o vfs-cli main.go && \
    CGO_ENABLED=0 GOOS=linux go build -o vfs-cli-prompt main_prompt.go

FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates jq curl bash zsh

# Copy binaries from builder
COPY --from=builder /app/cli/vfs-cli .
COPY --from=builder /app/cli/vfs-cli-prompt .

# Default to prompt version
CMD ["./vfs-cli-prompt"]
