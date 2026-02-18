SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

DGRAPH_ALPHA ?= http://localhost:8080
DGRAPH_GRPC  ?= localhost:9080
AUTO_INSTALL ?= false

.PHONY: help setup reset generate build test check docker-up docker-down \
        deps deps-go deps-docker deps-patched-mods \
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

deps: deps-go deps-docker deps-patched-mods ## Check all dependencies (tools + fork repos)

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

deps-patched-mods: ## Check patched fork repos exist at replace paths
	@echo "Checking patched module forks …"
	@check_fork() { \
		local path="$$1" url="$$2" branch="$$3" name="$$4"; \
		echo "  $$name: checking $$path …"; \
		if [ ! -d "$$path" ]; then \
			if [ "$(AUTO_INSTALL)" = "true" ]; then \
				echo "    $$path not found — cloning from $$url …"; \
				git clone "$$url" "$$path"; \
				echo "    switching to branch $$branch …"; \
				git -C "$$path" checkout "$$branch"; \
				echo "    $$name: cloned and on branch $$branch"; \
				return 0; \
			else \
				echo "    Error: $$path does not exist."; \
				echo "    Clone it: git clone $$url $$path"; \
				echo "    Or re-run with AUTO_INSTALL=true"; \
				return 1; \
			fi; \
		fi; \
		if [ ! -d "$$path/.git" ]; then \
			echo "    Error: $$path exists but is not a git repository."; \
			return 1; \
		fi; \
		local actual_url; \
		actual_url=$$(git -C "$$path" remote get-url origin 2>/dev/null || echo ""); \
		if [ "$$actual_url" != "$$url" ]; then \
			echo "    Warning: origin remote is $$actual_url"; \
			echo "    Expected: $$url"; \
			echo "    This may be the upstream repo instead of the fork."; \
			if [ "$(AUTO_INSTALL)" = "true" ]; then \
				echo "    Setting origin to fork URL …"; \
				git -C "$$path" remote set-url origin "$$url"; \
				echo "    Fetching from fork …"; \
				git -C "$$path" fetch origin; \
			else \
				echo "    Fix with: git -C $$path remote set-url origin $$url"; \
				return 1; \
			fi; \
		fi; \
		local actual_branch; \
		actual_branch=$$(git -C "$$path" branch --show-current 2>/dev/null || echo ""); \
		if [ "$$actual_branch" != "$$branch" ]; then \
			echo "    Warning: on branch '$$actual_branch', expected '$$branch'"; \
			if [ "$(AUTO_INSTALL)" = "true" ]; then \
				echo "    Fetching and switching to $$branch …"; \
				git -C "$$path" fetch origin "$$branch"; \
				git -C "$$path" checkout "$$branch"; \
			else \
				echo "    Fix with: git -C $$path checkout $$branch"; \
				return 1; \
			fi; \
		fi; \
		echo "    $$name: OK ($$path on branch $$actual_branch)"; \
	}; \
	check_fork "../dgman"         "https://github.com/mlwelles/dgman.git"         "master" "dgman" && \
	check_fork "../modusGraph"    "https://github.com/mlwelles/modusGraph.git"    "main"   "modusGraph" && \
	check_fork "../modusGraphGen" "https://github.com/mlwelles/modusGraphGen.git" "master" "modusGraphGen" && \
	echo "All patched module forks OK."

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
