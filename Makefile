COMPOSE := docker compose -f deploy/docker-compose.yml
GO       := go

.PHONY: help up down rebuild logs ps smoke test lint fmt tidy arch

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

up: ## Build and start the full stack
	$(COMPOSE) up -d --build

down: ## Stop stack (keep volumes)
	$(COMPOSE) down

rebuild: ## Rebuild and recreate app replicas
	$(COMPOSE) up -d --build --force-recreate app

logs: ## Tail app logs
	$(COMPOSE) logs -f app

ps: ## Show compose status
	$(COMPOSE) ps

smoke: ## Authenticated order + ClickHouse prove
	./scripts/smoke.sh

test: ## Unit tests
	$(GO) test ./...

lint: ## go vet
	$(GO) vet ./...

fmt: ## Format
	gofmt -w ./cmd

tidy: ## go mod tidy
	$(GO) mod tidy

arch: ## Print request-path reminder
	@sed -n '1,80p' docs/ARCHITECTURE.md
