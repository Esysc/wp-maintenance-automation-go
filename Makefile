.PHONY: build test clean run-api run-web run-cli run run-server dev fmt vet lint deps install help

BINARY_CLI = bin/wp-maintenance
BINARY_API = bin/wp-maintenance-api
BINARY_WEB = bin/wp-maintenance-web
BINARY_SERVER = bin/wp-maintenance-server

all: build

build: build-cli build-api build-web build-server

build-cli:
	@echo "Building CLI..."
	@mkdir -p bin
	go build -o $(BINARY_CLI) ./cmd/cli/

build-api:
	@echo "Building API server..."
	@mkdir -p bin
	go build -o $(BINARY_API) ./cmd/api/

build-web:
	@echo "Building Web server..."
	@mkdir -p bin
	go build -o $(BINARY_WEB) ./cmd/web/

build-server:
	@echo "Building combined server..."
	@mkdir -p bin
	go build -o $(BINARY_SERVER) ./cmd/server/

test:
	@echo "Running tests..."
	go test -v -race -count=1 ./...

test-cover:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

vet:
	go vet ./...

lint:
	golangci-lint run ./... || true

clean:
	rm -rf bin/ coverage.out coverage.html

run-api:
	@echo "Starting API server on port 8081..."
	go run ./cmd/api/

run-web:
	@echo "Starting Web server on port 8080..."
	go run ./cmd/web/

run-cli:
	@echo "Running CLI. Usage: go run ./cmd/cli/ help"
	@go run ./cmd/cli/ help

run-server:
	@echo "Starting combined server (API :8081 + Web :8080)..."
	go run ./cmd/server/

run: run-server

dev:
	@echo "Starting combined server..."
	@echo "API: http://localhost:8081"
	@echo "Web: http://localhost:8080"
	go run ./cmd/server/

fmt:
	go fmt ./...

deps:
	go mod tidy
	go mod download

install:
	@echo "Installing binaries..."
	go install ./cmd/cli/
	go install ./cmd/api/
	go install ./cmd/web/
	go install ./cmd/server/

help:
	@echo "WP Maintenance Automation Go"
	@echo ""
	@echo "Usage:"
	@echo "  make build        Build all binaries"
	@echo "  make build-server Build combined server binary"
	@echo "  make build-cli    Build CLI binary"
	@echo "  make build-api    Build API server binary"
	@echo "  make build-web    Build Web server binary"
	@echo "  make test         Run tests"
	@echo "  make test-cover   Run tests with coverage"
	@echo "  make vet          Run go vet"
	@echo "  make lint         Run linter"
	@echo "  make run-server   Run combined server (default)"
	@echo "  make run-api      Run API server only"
	@echo "  make run-web      Run Web server only"
	@echo "  make run-cli      Run CLI"
	@echo "  make dev          Run combined server"
	@echo "  make clean        Clean build artifacts"
	@echo "  make deps         Download dependencies"
	@echo "  make install      Install binaries to GOPATH/bin"
