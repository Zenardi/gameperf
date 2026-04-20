BINARY    := gameperf
BUILD_DIR := dist
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build run serve test test-race lint tidy clean install

## build: compile the binary into dist/
build:
	mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gameperf

## run: build and run 'diagnose' (pass extra args with ARGS="--llm")
run: build
	$(BUILD_DIR)/$(BINARY) diagnose $(ARGS)

## serve: build and start the Prometheus metrics server
serve: build
	$(BUILD_DIR)/$(BINARY) serve $(ARGS)

## install: install binary to GOPATH/bin (ensure $(go env GOPATH)/bin is in PATH)
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/gameperf
	@echo ""
	@echo "  Installed to $$(go env GOPATH)/bin/$(BINARY)"
	@if ! echo "$$PATH" | grep -q "$$(go env GOPATH)/bin"; then \
		echo ""; \
		echo "  WARNING: $$(go env GOPATH)/bin is not in your PATH."; \
		echo "  Add this to your shell config (~/.zshrc or ~/.bashrc):"; \
		echo ""; \
		echo "    export PATH=\$$PATH:$$(go env GOPATH)/bin"; \
		echo ""; \
		echo "  Then reload: source ~/.zshrc"; \
	fi

## test: run all tests
test:
	go test -v ./...

## test-race: run all tests with the race detector
test-race:
	go test -race -v ./...

## lint: run golangci-lint (install from https://golangci-lint.run/usage/install/)
lint:
	golangci-lint run ./...

## tidy: tidy and verify go.mod / go.sum
tidy:
	go mod tidy
	go mod verify

## clean: remove build artefacts
clean:
	rm -rf $(BUILD_DIR)

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
