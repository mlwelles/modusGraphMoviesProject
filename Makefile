SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

DGRAPH_ALPHA ?= http://localhost:8080
DGRAPH_GRPC  ?= localhost:9080

.PHONY: help setup reset generate build test check docker-up docker-down \
        deps deps-go deps-docker \
        fetch-data load-data drop-data dgraph-ready ensure-data

.DEFAULT_GOAL := help

# =============================================================================
# User-facing targets
# =============================================================================

help: ## Show this help message
	@echo ""
	@echo "Environment Variables:"
	@echo "  DGRAPH_ALPHA=<url>   Dgraph Alpha HTTP endpoint (default: http://localhost:8080)"
	@echo "  DGRAPH_GRPC=<addr>   Dgraph gRPC endpoint (default: localhost:9080)"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""

setup: deps docker-up load-data ## Full onboarding: check deps, start Dgraph, load data

reset: docker-up drop-data load-data ## Reset: drop all data, reload data

generate: ## Run modusGraphGen (client library + CLI)
	go generate ./movies

build: ## Build the movies CLI binary
	go build -o bin/movies ./cmd/movies

test: ensure-data dgraph-ready ## Run the test suite (self-healing: bootstraps Dgraph if needed)
	go test ./...

check: ## Run go vet on all packages
	go vet ./...

docker-up: deps-docker ## Start Dgraph containers
	@docker compose up -d

docker-down: ## Stop Dgraph containers
	@if docker compose ps --status running 2>/dev/null | grep -q modus-movies-dgraph; then \
		docker compose down; \
	fi

deps: deps-go deps-docker ## Check all tool dependencies

# =============================================================================
# Internal targets
# =============================================================================

deps-go:
	@command -v go >/dev/null 2>&1 || { \
		echo "Error: go is not installed."; \
		echo ""; \
		echo "Install Go from: https://go.dev/dl/"; \
		exit 1; \
	}

deps-docker:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Error: Docker is not installed."; \
		echo ""; \
		echo "To install Docker:"; \
		echo ""; \
		if [ "$$(uname)" = "Darwin" ]; then \
			echo "  macOS:"; \
			echo "    brew install --cask docker"; \
			echo "    # or download from"; \
			echo "    # https://docs.docker.com/desktop/install/mac-install/"; \
		elif [ "$$(uname)" = "Linux" ]; then \
			echo "  Linux (Debian/Ubuntu):"; \
			echo "    sudo apt-get update"; \
			echo "    sudo apt-get install -y docker.io docker-compose-plugin"; \
			echo "    sudo systemctl start docker"; \
			echo "    sudo systemctl enable docker"; \
			echo "    sudo usermod -aG docker $$USER"; \
		else \
			echo "  Windows:"; \
			echo "    Download from: https://docs.docker.com/desktop/install/windows-install/"; \
		fi; \
		echo ""; \
		exit 1; \
	fi
	@if ! docker compose version >/dev/null 2>&1; then \
		echo "Error: 'docker compose' command not available. Please ensure Docker Compose v2 is installed."; \
		exit 1; \
	fi

dgraph-ready: docker-up
	@timeout=60; while ! curl -s $(DGRAPH_ALPHA)/health | grep -q '"status":"healthy"'; do \
		sleep 1; \
		timeout=$$((timeout - 1)); \
		if [ $$timeout -le 0 ]; then echo "Timeout waiting for Dgraph health endpoint (GET $(DGRAPH_ALPHA)/health)"; exit 1; fi; \
	done

fetch-data:
	@bash tasks/fetch-data.sh

load-data: fetch-data dgraph-ready ## Load the 1M movie dataset into Dgraph
	@bash tasks/load-data.sh

drop-data: dgraph-ready ## Drop all data from Dgraph
	@bash tasks/drop-data.sh

ensure-data: dgraph-ready
	@COUNT=$$(curl -s -X POST $(DGRAPH_ALPHA)/query -H "Content-Type: application/dql" \
		-d '{ count(func: type(Film)) { total: count(uid) } }' \
		| grep -oE '"total":[0-9]+' | grep -oE '[0-9]+' || echo "0"); \
	if [ "$$COUNT" -eq 0 ] 2>/dev/null; then \
		echo "No film data found — loading…"; \
		$(MAKE) load-data; \
	fi
