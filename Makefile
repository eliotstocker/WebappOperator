# Build configuration
BINARY_NAME ?= bin/operator
IMAGE_NAME ?= ghcr.io/eliotstocker/webapp-operator
TAG ?= latest

.PHONY: all build test test-coverage bench run docker-build clean

all: test build

## build: Compiles the unified webapp-operator binary
build:
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
