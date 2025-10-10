
SHELL := /bin/bash

gen:
	buf generate

dev: gen up

up:
	docker compose -f docker/docker-compose.yaml up -d --build

down:
	docker compose -f docker/docker-compose.yaml down

migrate:
	@echo "Apply migrations (placeholder). Use a tool like golang-migrate or goose."
	@echo "psql -h localhost -p 5433 -U postgres -d sentinel -f migrations/001_init.sql"

test:
	go test ./...

lint:
	buf lint
	go vet ./...

build:
	go build ./cmd/sentinel-api
	go build ./cmd/sentinel-worker

logs:
	docker compose -f docker/docker-compose.yaml logs -f
