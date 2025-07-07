# Makefile for PJSUA test tools

.PHONY: all test test-verbose test-race coverage lint clean examples

# Default target
all: test

# Run tests
test:
	go test ./...

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run tests with race detector
test-race:
	go test -race ./...

# Generate test coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Build examples
examples:
	go build -o bin/basic_usage ./examples/basic_usage.go
	go build -o bin/event_handling ./examples/event_handling.go
	go build -o bin/automated_test ./examples/automated_test.go
	go build -o bin/advanced_config ./examples/advanced_config.go
	@echo "Examples built in bin/"

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean ./...

# Install the package
install:
	go install ./...

# Run specific example
run-basic:
	go run ./examples/basic_usage.go

run-events:
	go run ./examples/event_handling.go

run-test:
	go run ./examples/automated_test.go

run-config:
	go run ./examples/advanced_config.go

# Generate documentation
doc:
	godoc -http=:6060 &
	@echo "Documentation server started at http://localhost:6060/pkg/github.com/arzzra/soft_phone/test_tools/pjsua/"

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# Run all checks
check: fmt vet test lint