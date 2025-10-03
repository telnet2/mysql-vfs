FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o vfs-cli ./cli

FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache ca-certificates jq curl bash

COPY --from=builder /app/vfs-cli .

ENTRYPOINT ["./vfs-cli"]
