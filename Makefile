# Build configuration
BINARY_NAME ?= bin/operator
IMAGE_NAME ?= ghcr.io/eliotstocker/webapp-operator
TAG ?= latest
CRAWLER_UA_URL ?= https://raw.githubusercontent.com/monperrus/crawler-user-agents/master/crawler-user-agents.json

.PHONY: all fetch-bots build test test-coverage bench run docker-build clean

all: test build

## fetch-bots: Downloads the latest crawler-user-agents.json for embedding at build time
fetch-bots:
	curl -fsSL $(CRAWLER_UA_URL) -o internal/server/crawler-user-agents.json

## build: Compiles the unified webapp-operator binary
build: fetch-bots
	@mkdir -p bin
	CGO_ENABLED=0 go build -o $(BINARY_NAME) ./cmd/operator

## test: Runs all unit and end-to-end tests
test:
	go test -v ./...

## test-coverage: Runs tests with coverage and displays report
test-coverage:
	go test -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/... ./internal/... ./test/e2e/...
	@go tool cover -func=coverage.out
	@rm -f coverage.out

## bench: Runs HTTP server throughput and memory benchmarks
bench:
	go test -bench=. -benchmem -run=^$$ ./internal/server

## run: Runs the operator locally (useful for development)
run:
	go run ./cmd/operator --leader-elect=false --cache-dir=/tmp/webapp-cache

## docker-build: Builds the container image locally
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) .

## clean: Removes build artifacts and temporary files
clean:
	rm -rf bin/ coverage.out
