.PHONY: up down baseline test

up:
	docker compose up -d --wait

down:
	docker compose down

baseline:
	./scripts/run-baseline.sh

test:
	go test ./...
