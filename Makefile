.PHONY: all build test run tidy docker-up docker-down

all: build test

tidy:
	go mod tidy

build:
	go build -v -o bin/ems ./cmd/ems

test:
	go test -v -race -cover ./...

run:
	go run ./cmd/ems

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v

docker-prod-up:
	docker compose -f compose.prod.yaml up -d --build

docker-prod-down:
	docker compose -f compose.prod.yaml down

backup:
	./scripts/backup.sh
