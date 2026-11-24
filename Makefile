# One-word commands for humans and AI agents. `make help` lists them.
# Works on Linux, macOS and Windows via WSL2.

TAILWIND_VERSION ?= 4.3.5
TAILWIND := ./bin/tailwindcss
HTMX ?= 4.0.2
ALPINE ?= 3.17.5
COMPOSE := docker compose
# Dev Postgres host port (compose.yaml publishes it; must match DATABASE_URL in .env)
DB_PORT ?= 5433
export DB_PORT

.DEFAULT_GOAL := help
.PHONY: help setup dev generate css build run test test-integration test-e2e lint check fmt migrate \
	up down logs db-shell docker-build keygen vendor clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

setup: ## First-time setup: tools, deps, .env with fresh secrets
	@./scripts/setup.sh $(TAILWIND_VERSION)

dev: ## Start Postgres + Mailpit in Docker, run the app with hot reload
	$(COMPOSE) up -d db mailpit
	go tool air

generate: ## Generate templ Go code + CSS
	go tool templ generate
	$(MAKE) --no-print-directory css

css: ## Build minified CSS with the Tailwind standalone CLI
	$(TAILWIND) -i static/css/input.css -o static/css/app.css --minify

build: generate ## Build a static binary into bin/app
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/app ./cmd/app

run: build ## Build and run (web + worker)
	./bin/app

test: generate ## Unit tests (fast, no DB)
	go test -count=1 ./...

test-integration: generate ## Tests that need Postgres (uses TEST_DATABASE_URL)
	$(COMPOSE) up -d db
	TEST_DATABASE_URL=$${TEST_DATABASE_URL:-postgres://app:app@localhost:$(DB_PORT)/app_test?sslmode=disable} \
		go test -count=1 -race -tags=integration ./...

test-e2e: ## Browser tests in Google Chrome (Playwright + axe accessibility). App must be running.
	@command -v google-chrome >/dev/null || { echo "Google Chrome missing. Install once: cd e2e && sudo npx playwright install --with-deps chrome"; exit 1; }
	cd e2e && npm install --silent && npx playwright test

lint: ## Vet, format check and known-vulnerability scan
	@test -z "$$(gofmt -l cmd internal static)" || (echo "gofmt needed:"; gofmt -l cmd internal static; exit 1)
	go tool templ fmt -fail .
	go vet ./...
	go tool govulncheck ./...

check: generate lint test ## Everything CI runs. Run before every commit.

fmt: ## Format Go and templ files
	gofmt -w cmd internal static
	go tool templ fmt .

migrate: ## Apply database migrations
	go run ./cmd/app migrate

keygen: ## Print fresh secrets for .env
	@go run ./cmd/app keygen

up: ## Run the FULL stack in Docker (like production, on your machine)
	$(COMPOSE) --profile full up -d --build

down: ## Stop all containers
	$(COMPOSE) --profile full down

logs: ## Follow app logs (full stack)
	$(COMPOSE) logs -f app

db-shell: ## psql into the dev database
	$(COMPOSE) exec db psql -U app app

docker-build: ## Build the production image
	docker build -t app:latest .

vendor: ## Update vendored htmx / Alpine (HTMX=x.y.z ALPINE=x.y.z)
	@./scripts/vendor.sh $(HTMX) $(ALPINE)

clean: ## Remove build output
	rm -rf bin/app tmp static/css/app.css
	find . -name '*_templ.go' -delete
