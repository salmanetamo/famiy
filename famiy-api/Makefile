# Run from famiy-api/. Needs: go, docker compose, migrate, golangci-lint.
-include .env
export

COMPOSE := docker compose -f ../deploy/docker-compose.yml --env-file .env

.PHONY: dev db-up db-down up down build test vet lint fmt check migrate-up migrate-down migrate-new

dev: db-up migrate-up ## run API on host against the dockerised db
	go run ./cmd/server

db-up:
	$(COMPOSE) up -d --wait db

db-down:
	$(COMPOSE) stop db

up: ## full containerised stack (db + api)
	$(COMPOSE) --profile app up -d --build --wait

down:
	$(COMPOSE) --profile app down

build:
	go build ./...

test: db-up ## integration tests create a throwaway database per test on the dev server
	TEST_DATABASE_URL="$(DATABASE_URL)" go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

check: fmt vet lint test

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down: ## roll back ONE migration
	migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-new: ## usage: make migrate-new name=add_foo
	migrate create -ext sql -dir migrations -seq $(name)
