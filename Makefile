SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

DGRAPH_ALPHA ?= http://localhost:8080
DGRAPH_GRPC  ?= localhost:9080
AUTO_INSTALL ?= false

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
	@echo "  DGRAPH_ALPHA=<url>     Dgraph Alpha HTTP endpoint (default: http://localhost:8080)"
	@echo "  DGRAPH_GRPC=<addr>     Dgraph gRPC endpoint (default: localhost:9080)"
	@echo "  AUTO_INSTALL=true      Auto-install missing deps instead of printing instructions"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""

setup: deps docker-up load-data ## Full onboarding: check deps, start Dgraph, load data

reset: docker-up drop-data load-data ## Reset: drop all data, reload data

generate: ## Run modusGraphGen (client library + CLI)
	go generate ./movies

build: ## Build the movies CLI binary
	go build -o bin/movies ./movies/cmd/movies

test: ensure-data dgraph-ready ## Run the test suite (self-healing: bootstraps Dgraph if needed)
	DGRAPH_TEST_ADDR=$(DGRAPH_GRPC) go test ./...

check: ## Run go vet on all packages
	go vet ./...

docker-up: deps-docker ## Start Dgraph containers
	@docker compose up -d

docker-down: ## Stop Dgraph containers
	@if docker compose ps --status running 2>/dev/null | grep -q modus-movies-dgraph; then \
		docker compose down; \
	fi

deps: deps-go deps-docker ## Check all dependencies

# =============================================================================
# Internal targets
# =============================================================================

deps-go:
	@echo "Checking Go …"
	@if command -v go >/dev/null 2>&1; then \
		echo "  go: $$(go version)"; \
	else \
		if [ "$(AUTO_INSTALL)" = "true" ]; then \
			if [ "$$(uname)" = "Darwin" ]; then \
				echo "  go: not found — installing via Homebrew …"; \
				brew install go; \
				echo "  go: $$(go version)"; \
			else \
				echo "Error: go is not installed and AUTO_INSTALL is not supported for $$(uname)."; \
				echo "  Install Go from: https://go.dev/dl/"; \
				exit 1; \
			fi; \
		else \
			echo "Error: go is not installed."; \
			echo ""; \
			echo "  Install Go from: https://go.dev/dl/"; \
			if [ "$$(uname)" = "Darwin" ]; then \
				echo "  macOS: brew install go"; \
			fi; \
			echo ""; \
			echo "  Or re-run with AUTO_INSTALL=true to install automatically (macOS)."; \
			exit 1; \
		fi; \
	fi

deps-docker:
	@echo "Checking Docker …"
	@if command -v docker >/dev/null 2>&1; then \
		echo "  docker: $$(docker --version)"; \
	else \
		if [ "$(AUTO_INSTALL)" = "true" ]; then \
			if [ "$$(uname)" = "Darwin" ]; then \
				echo "  docker: not found — installing via Homebrew …"; \
				brew install --cask docker; \
				echo "  docker: $$(docker --version)"; \
			elif [ "$$(uname)" = "Linux" ]; then \
				echo "  docker: not found — installing via apt …"; \
				sudo apt-get update; \
				sudo apt-get install -y docker.io docker-compose-plugin; \
				sudo systemctl start docker; \
				sudo systemctl enable docker; \
				sudo usermod -aG docker $$USER; \
				echo "  docker: $$(docker --version)"; \
				echo "  NOTE: You may need to log out and back in for group changes to take effect."; \
			else \
				echo "Error: Docker is not installed and AUTO_INSTALL is not supported for $$(uname)."; \
				echo "  Download from: https://docs.docker.com/desktop/install/"; \
				exit 1; \
			fi; \
		else \
			echo "Error: Docker is not installed."; \
			echo ""; \
			if [ "$$(uname)" = "Darwin" ]; then \
				echo "  macOS:"; \
				echo "    brew install --cask docker"; \
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
			echo "  Or re-run with AUTO_INSTALL=true to install automatically (macOS, Linux)."; \
			exit 1; \
		fi; \
	fi
	@echo "Checking Docker Compose …"
	@if docker compose version >/dev/null 2>&1; then \
		echo "  docker compose: $$(docker compose version --short)"; \
	else \
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
