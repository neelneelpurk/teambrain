# teambrain — developer task runner.
# Run `make help` for the list of targets.

GOPATH_BIN       := $(shell go env GOPATH)/bin
GOLANGCI_VERSION := v2.12.2
BINARY           := teambrain
MCP_BINARY       := teambrain-mcp
PKG              := github.com/neelneelpurk/teambrain
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT           ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE             ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS          := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the CLI binary into ./bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/teambrain

.PHONY: build-mcp
build-mcp: ## Build the Obsidian MCP server into ./bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(MCP_BINARY) ./cmd/teambrain-mcp

.PHONY: build-all
build-all: build build-mcp ## Build both binaries into ./bin

.PHONY: install
install: ## go install both binaries into GOPATH/bin
	go install -ldflags "$(LDFLAGS)" ./cmd/teambrain ./cmd/teambrain-mcp

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: race
race: ## Run all tests with the race detector
	go test ./... -race

.PHONY: cover
cover: ## Run tests with coverage and print the total
	go test ./... -race -coverprofile=coverage.txt -covermode=atomic
	go tool cover -func=coverage.txt | tail -1

.PHONY: cover-check
cover-check: ## Enforce the per-package internal coverage floor (>=80%)
	./scripts/check-coverage.sh 80

.PHONY: cover-html
cover-html: cover ## Open an HTML coverage report
	go tool cover -html=coverage.txt -o coverage.html

.PHONY: fmt
fmt: ## Format all Go code
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any file needs gofmt
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "needs gofmt:"; echo "$$unformatted"; exit 1; fi

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	$(GOPATH_BIN)/golangci-lint run

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

.PHONY: tools
tools: ## Install dev tools (golangci-lint) pinned to the CI version
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
		| sh -s -- -b "$(GOPATH_BIN)" $(GOLANGCI_VERSION)

.PHONY: ci
ci: fmt-check vet lint race cover-check ## Run the full gate set (mirrors CI)

.PHONY: clean
clean: ## Remove build and coverage artifacts
	rm -rf bin dist coverage.txt coverage.html
