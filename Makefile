.DEFAULT_GOAL := help

BIN := bin/spotify-mcp

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the server binary into ./bin
	go build -o $(BIN) ./cmd/spotify-mcp

.PHONY: auth
auth: build ## One-time interactive Spotify login (opens a browser)
	$(BIN) auth

.PHONY: run
run: build ## Run the MCP server over stdio
	$(BIN) serve

.PHONY: test
test: ## Run tests
	go test ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (must be installed)
	golangci-lint run

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy
