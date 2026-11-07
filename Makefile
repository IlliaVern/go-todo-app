COMPOSE := docker compose -f docker/compose.yaml $(if $(wildcard .env),--env-file .env)

.DEFAULT_GOAL := help

.PHONY: help up down infra logs run build test vet fmt check vuln

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z]+:.*## / {printf "  %-8s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up: ## Build and start PostgreSQL, migrations and the API in Docker
	$(COMPOSE) up --build --detach

down: ## Stop the Docker stack (add ARGS=-v to also drop the database volume)
	$(COMPOSE) down $(ARGS)

infra: ## Start only PostgreSQL and apply migrations, for `make run`
	$(COMPOSE) up --detach migrate

logs: ## Follow Docker stack logs
	$(COMPOSE) logs --follow

run: ## Run the API on the host (needs `make infra` and a .env file)
	go run ./cmd

build: ## Build the API binary into bin/
	go build -trimpath -o bin/todo-app ./cmd

test: ## Run tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format Go sources
	gofmt -w .

check: vet test ## Run all local checks (gofmt, vet, tests)
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }

vuln: ## Scan dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
