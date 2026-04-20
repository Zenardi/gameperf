BINARY := gameperf
BUILD_DIR := dist

.PHONY: build run test clean install

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/gameperf

run: build
	$(BUILD_DIR)/$(BINARY) diagnose

install:
	go install ./cmd/gameperf

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)
