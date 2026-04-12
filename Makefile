.PHONY: build test vet fmt run tidy help

APP    := overload-party-matchmaking
MODULE := github.com/kenyamaneko/$(APP)

build: ## Build Docker image
	docker build -t $(APP) .

test: ## Run unit tests
	go test ./... -count=1 -race

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy dependencies
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

run: ## Run matchmaking server locally
	go run ./cmd/server

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
