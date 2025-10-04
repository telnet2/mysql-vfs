BIN_DIR := bin
CLI_NAME := vfscli


.PHONY: build clean

build:
	@mkdir -p $(BIN_DIR)
	GO111MODULE=on go build -o $(BIN_DIR)/$(CLI_NAME) ./cmd/vfscli

clean:
	@rm -f $(BIN_DIR)/$(CLI_NAME)
