BINARY := devspanner
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install run test cover fmt fmt-check vet lint check clean snapshot tidy help

all: check build ## Run checks, then build

build: ## Build the binary into ./$(BINARY)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: ## Install into $GOBIN / $GOPATH/bin
	go install -ldflags "$(LDFLAGS)" .

run: ## Build and run
	go run -ldflags "$(LDFLAGS)" .

test: ## Run tests with the race detector
	go test -race ./...

cover: ## Run tests and open a coverage report
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-clean
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt-clean:"; echo "$$out"; exit 1; fi

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (https://golangci-lint.run/welcome/install/)
	golangci-lint run

check: fmt-check vet lint test ## Run everything CI runs

snapshot: ## Build a local release snapshot with GoReleaser (no publish)
	goreleaser release --snapshot --clean

tidy: ## Tidy go.mod / go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out
	rm -rf dist

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
