.PHONY: help build up down logs test clean migrate

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build all Docker images
	docker-compose build

up: ## Start all services
	docker-compose up -d
	@echo "Services starting... waiting for health checks..."
	@sleep 10
	@docker-compose ps

up-cli: ## Start all services including CLI
	docker-compose --profile cli up -d
	@echo "Services starting... waiting for health checks..."
	@sleep 10
	docker-compose exec cli /bin/bash

down: ## Stop all services
	docker-compose down

logs: ## Follow logs for all services
	docker-compose logs -f

logs-vfs: ## Follow logs for VFS service
	docker-compose logs -f vfs-service

logs-worker: ## Follow logs for event worker
	docker-compose logs -f event-worker

test: ## Run integration tests
	go test -v ./...

test-integration: ## Run integration tests against docker-compose stack
	@echo "Integration tests will be implemented in Phase 6"

clean: ## Clean up Docker resources
	docker-compose down -v
	docker system prune -f

migrate: ## Run database migrations
	@echo "Migrations run automatically on service startup"

tidy: ## Tidy Go modules
	go mod tidy

fmt: ## Format Go code
	go fmt ./...

lint: ## Run linter
	golangci-lint run ./...

dev: ## Start services in development mode
	docker-compose up

cli: ## Connect to CLI container
	docker-compose exec cli /bin/bash

cli-build: ## Build CLI
	cd cli && go build -o ../vfs-cli .

db-shell: ## Connect to MySQL shell
	docker-compose exec mysql mysql -uroot -proot vfs

s3-init: ## Initialize S3 bucket in LocalStack
	docker-compose exec localstack awslocal s3 mb s3://vfs-storage || true
	docker-compose exec localstack awslocal s3 ls

status: ## Show service status
	docker-compose ps
