APP_NAME   := bugbuster
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS    := -X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)

.PHONY: build run test clean install lint fmt release

# Build
build:
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) ./cmd/bugbuster/

# Build and run
run: build
	./$(APP_NAME)

# Run tests
test:
	go test -v -race ./...

# Test coverage
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -f $(APP_NAME) coverage.out coverage.html

# Install to GOPATH/bin
install: build
	cp $(APP_NAME) $(GOPATH)/bin/$(APP_NAME)

# Run linters
lint:
	golangci-lint run ./...

# Format code
fmt:
	gofmt -w .
	goimports -w .

# Release (example: make release VERSION=v1.0.0)
release: build
	@echo "Built $(APP_NAME) v$(VERSION) ($(GIT_COMMIT))"