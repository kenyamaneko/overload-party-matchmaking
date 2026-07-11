.PHONY: build test test-integration vet fmt run tidy down generate-types help

APP    := overload-party-matchmaking
MODULE := github.com/kenyamaneko/$(APP)

build: ## Build Docker image
	docker build -t $(APP) .

test: ## Run unit tests (no external deps)
	go test ./... -count=1 -race

test-integration: ## Run unit + integration tests (Testcontainers starts Valkey; requires Docker)
	go test ./... -count=1 -race -tags=integration

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy dependencies
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

run: ## Run the full local stack (app + infra) in compose; edit source and restart `matchmaking` to reload
	GOWORK=off GOPRIVATE=github.com/kenyamaneko/* go mod download
	HOST_GOMODCACHE=$$(go env GOMODCACHE) docker compose up

down: ## Stop the local stack and remove volumes
	HOST_GOMODCACHE=$$(go env GOMODCACHE) docker compose down -v

generate-types: ## Re-generate packages/api-matchmaking/{openapi,asyncapi}_gen.go from data/{openapi,asyncapi}.yaml
	scripts/generate_types.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
