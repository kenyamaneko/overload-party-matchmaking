.PHONY: build test vet fmt run tidy up down run-local generate-types help

APP    := overload-party-matchmaking
MODULE := github.com/kenyamaneko/$(APP)

build: ## Build Docker image
	docker build -t $(APP) .

test: up ## Run tests against local Valkey (auto-starts deps)
	go test ./... -count=1 -race

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy dependencies
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

run: ## Run matchmaking server locally
	go run ./cmd/server

up: ## Start local Redis + Pub/Sub emulator
	# redis / pubsub は healthcheck 持ちなので --wait で Healthy を待つ。
	# pubsub-init は one-shot (exit 0 で完了) なので別ステップで同期的に実行する
	# (--wait は完了を unhealthy と誤判定するため)。
	docker compose up -d --wait redis pubsub
	docker compose up pubsub-init

down: ## Stop local dependencies
	docker compose down -v

run-local: up ## Run server against local deps (auto-starts Valkey + Pub/Sub emulator)
	set -a && . ./.env.local && set +a && go run ./cmd/server

generate-types: ## Re-generate packages/api-matchmaking/{openapi,asyncapi}_gen.go from data/{openapi,asyncapi}.yaml
	scripts/generate_types.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
