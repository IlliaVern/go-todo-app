COMPOSE := docker compose -f docker/compose.yaml $(if $(wildcard .env),--env-file .env)

.DEFAULT_GOAL := help

.PHONY: help up down infra logs run build test test-unit e2e vet fmt check vuln

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

run: ## Run the API on the host (needs `make infra` and a .env file)
	go run ./cmd/api

build: ## Build the API binary into bin/
	go build -trimpath -o bin/todo-app ./cmd/api

test: ## Run all tests with the race detector (Docker-backed tests skip if Docker is down)
	go test -race ./...

test-unit: ## Run tests without Docker (skips repository and end-to-end tests)
	go test -race -short ./...

e2e: ## Run the end-to-end tests against the stack started by `make up`
	E2E_BASE_URL=http://localhost:8000 go test -count=1 ./internal/e2e/

vet: ## Run go vet
	go vet ./...

fmt: ## Format Go sources
	gofmt -w .

check: vet ## Run all local checks (gofmt, vet, all tests; fails if Docker is unavailable)
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }
	REQUIRE_DOCKER=1 go test -race ./...

vuln: ## Scan dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
