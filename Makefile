.PHONY: all build test lint docker-up docker-down migrate run-scheduler run-worker

BINARY_SCHEDULER=bin/scheduler
BINARY_WORKER=bin/worker

all: build

build:
	go build -o $(BINARY_SCHEDULER) ./cmd/scheduler
	go build -o $(BINARY_WORKER) ./cmd/worker

run-scheduler:
	go run ./cmd/scheduler

run-worker:
	go run ./cmd/worker

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

docker-up:
	docker compose -f deployments/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker-compose.yml down -v

migrate:
	psql $$POSTGRES_URL -f internal/store/migrations/001_init.sql

seed:
	go run scripts/seed.go

clean:
	rm -rf bin/ coverage.out coverage.html
