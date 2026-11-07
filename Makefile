COMPOSE := docker compose -f docker/compose.yaml $(if $(wildcard .env),--env-file .env)
# Developer tools are pinned in tools/go.mod, apart from the application's dependencies.
TOOL := go tool -modfile=tools/go.mod

.DEFAULT_GOAL := help

.PHONY: help up down infra logs run build test test-unit e2e lint fmt check vuln

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z0-9-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up: ## Build and start PostgreSQL, migrations and the API in Docker
	$(COMPOSE) up --build --detach

down: ## Stop the Docker stack (add ARGS=-v to also drop the database volume)
	$(COMPOSE) down $(ARGS)

infra: ## Start only PostgreSQL and apply migrations, for `make run`
	$(COMPOSE) up --detach migrate

logs: ## Follow Docker stack logs
	$(COMPOSE) logs --follow

run: ## Run the API on the host with .env exported (needs `make infra`)
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; go run ./cmd/api

build: ## Build the API binary into bin/
	go build -trimpath -o bin/todo-app ./cmd/api

test: ## Run all tests with the race detector (Docker-backed tests skip if Docker is down)
	go test -race ./...

test-unit: ## Run tests without Docker (skips repository and end-to-end tests)
	go test -race -short ./...

e2e: ## Run the end-to-end tests against the stack started by `make up`
	E2E_BASE_URL=http://localhost:8000 go test -count=1 ./internal/e2e/

lint: ## Run golangci-lint (formatting, go vet and the linters in .golangci.yml)
	$(TOOL) golangci-lint run ./...

fmt: ## Format Go sources and group imports
	$(TOOL) golangci-lint fmt ./...

check: lint ## Run all local checks (lint, all tests; fails if Docker is unavailable)
	REQUIRE_DOCKER=1 go test -race ./...

vuln: ## Scan dependencies for known vulnerabilities
	$(TOOL) govulncheck ./...
