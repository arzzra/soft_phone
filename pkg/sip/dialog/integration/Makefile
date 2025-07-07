# Makefile for SIP Dialog Integration Tests

.PHONY: help up down test test-dialog test-refer test-all logs clean

# Default target
help:
	@echo "Available targets:"
	@echo "  up          - Start OpenSIPS and call-api containers"
	@echo "  down        - Stop and remove containers"
	@echo "  test        - Run all integration tests"
	@echo "  test-dialog - Run dialog tests only"
	@echo "  test-refer  - Run REFER tests only"
	@echo "  test-client - Run client unit tests"
	@echo "  logs        - Show container logs"
	@echo "  clean       - Stop containers and clean up"

# Start containers
up:
	docker-compose up -d
	@echo "Waiting for services to start..."
	@sleep 5
	@docker-compose ps

# Stop containers
down:
	docker-compose down

# Run all tests
test: up
	go test ./tests/... -v -timeout 60s
	@$(MAKE) down

# Run dialog tests only
test-dialog: up
	go test ./tests/ -run TestBasic -v -timeout 30s
	go test ./tests/ -run TestCallReject -v -timeout 30s
	go test ./tests/ -run TestConcurrent -v -timeout 30s
	go test ./tests/ -run TestDialogState -v -timeout 30s
	@$(MAKE) down

# Run REFER tests only
test-refer: up
	go test ./tests/ -run TestBlindTransfer -v -timeout 30s
	go test ./tests/ -run TestAttendedTransfer -v -timeout 30s
	go test ./tests/ -run TestReferWith -v -timeout 30s
	go test ./tests/ -run TestReferRejection -v -timeout 30s
	@$(MAKE) down

# Run client unit tests
test-client:
	go test ./client/... -v

# Show logs
logs:
	docker-compose logs -f

# Show OpenSIPS logs only
logs-opensips:
	docker-compose logs -f opensips

# Show call-api logs only
logs-callapi:
	docker-compose logs -f call-api

# Clean up
clean: down
	docker-compose rm -f
	@echo "Cleanup complete"

# Development mode - keep containers running
dev-up:
	docker-compose up -d
	@echo "Services started in development mode"
	@echo "Run 'make dev-test' to run tests without stopping containers"

# Run tests in dev mode (without stopping containers)
dev-test:
	go test ./tests/... -v -timeout 60s

# Check if services are healthy
health:
	@docker-compose ps
	@echo ""
	@echo "Testing WebSocket connection..."
	@curl -s -o /dev/null -w "Call-API WebSocket: %{http_code}\n" http://localhost:5059 || echo "Call-API WebSocket: Connection failed"