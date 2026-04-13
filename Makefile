.PHONY: build test vet fmt run tidy up down run-local help

APP    := overload-party-matchmaking
MODULE := github.com/kenyamaneko/$(APP)

build: ## Build Docker image
	docker build -t $(APP) .

test: up ## Run tests against local Valkey (auto-starts deps)
	# DB 1 を使うのは、run-local が使う DB 0 (.env.local) と分離するため。
	# テストは毎回 FLUSHDB するので、サーバのキューと同居すると吹き飛ばしてしまう。
	TEST_REDIS_URL=redis://localhost:6379/1 go test ./... -count=1 -race

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy dependencies
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

run: ## Run matchmaking server locally
	go run ./cmd/server

up: ## Start local Redis + Pub/Sub emulator
	docker compose up -d --wait

down: ## Stop local dependencies
	docker compose down -v

run-local: up ## Run server against local deps (auto-starts Valkey + Pub/Sub emulator)
	set -a && . ./.env.local && set +a && go run ./cmd/server

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
