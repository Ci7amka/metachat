.PHONY: proto build run migrate test clean lint

# Build all services
build:
	go build -o bin/gateway ./services/gateway
	go build -o bin/auth ./services/auth
	go build -o bin/messaging ./services/messaging

# Run with docker-compose
run:
	docker-compose up --build

# Run in background
run-detached:
	docker-compose up --build -d

# Stop all services
stop:
	docker-compose down

# Run database migrations
migrate:
	docker-compose up migrate

# Run migrations down
migrate-down:
	docker-compose run --rm migrate -path /migrations -database "postgres://metachat:metachat_secret@postgres:5432/metachat?sslmode=disable" down

# Run tests
test:
	go test ./... -v

# Clean build artifacts
clean:
	rm -rf bin/
	docker-compose down -v

# Lint
lint:
	golangci-lint run ./...

# Proto (no-op since we use HTTP/JSON instead of gRPC)
proto:
	@echo "Using HTTP/JSON for inter-service communication — no protobuf generation needed"

# Tidy go modules
tidy:
	go mod tidy
