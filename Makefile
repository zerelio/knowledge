# Zerelio Makefile
# Usage: make [target]
#
# Common targets:
#   make build          — build the zerelio server binary
#   make plugins        — build all 24 discovery plugin binaries
#   make all            — build server + all plugins
#   make keys           — generate Ed25519 JWT key pair
#   make dev            — start dev stack (docker compose)
#   make test           — run all tests with race detector
#   make lint           — run staticcheck
#   make clean          — remove dist/

BINARY   := dist/zerelio
DIST     := dist
PLUGIN_OUT := $(DIST)/plugins

# Plugin repos are siblings of this repo by default.
# Override: make plugins PLUGIN_SRC=/path/to/plugins/parent
PLUGIN_SRC ?= ..

# All 24 plugin names (matches repo names: plugin-{name})
PLUGINS := aws k8s azure gcp github slack grafana jira confluence datadog \
           newrelic elasticsearch mongodb mysql postgresql cloudflare \
           servicenow vsphere prometheus codegraph dns pagerduty

# Target OS/arch for plugin binaries (can override for cross-compile)
GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

GO_FLAGS := CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH)
BUILD    := $(GO_FLAGS) go build -buildvcs=false -ldflags="-s -w"

.PHONY: all build plugins keys dev test lint clean help

all: build plugins ## Build server + all plugins

build: ## Build the zerelio server binary → dist/zerelio
	@mkdir -p $(DIST)
	$(BUILD) -o $(BINARY) ./cmd/zerelio
	@echo "→ $(BINARY)"

plugins: ## Build all discovery plugin binaries → dist/plugins/
	@mkdir -p $(PLUGIN_OUT)
	@failed=0; \
	for name in $(PLUGINS); do \
		src=$(PLUGIN_SRC)/plugin-$$name; \
		out=$(abspath $(PLUGIN_OUT))/plugin-$$name; \
		if [ ! -d "$$src" ]; then \
			echo "  SKIP  plugin-$$name ($$src not found)"; \
			continue; \
		fi; \
		printf "  BUILD plugin-%-20s" "$$name"; \
		if (cd $$src && $(BUILD) -o $$out . 2>/tmp/plugin-build-err); then \
			echo "→ $$out"; \
		else \
			echo "FAILED"; \
			cat /tmp/plugin-build-err; \
			failed=$$((failed+1)); \
		fi; \
	done; \
	if [ $$failed -gt 0 ]; then \
		echo ""; \
		echo "ERROR: $$failed plugin(s) failed to build"; \
		exit 1; \
	fi
	@echo ""
	@echo "Plugins built: $$(ls $(PLUGIN_OUT) | wc -l | tr -d ' ') → $(PLUGIN_OUT)/"

keys: ## Generate Ed25519 JWT key pair → dist/jwt_{private,public}.pem
	@mkdir -p $(DIST)
	openssl genpkey -algorithm ed25519 -out $(DIST)/jwt_private.pem
	openssl pkey -in $(DIST)/jwt_private.pem -pubout -out $(DIST)/jwt_public.pem
	chmod 600 $(DIST)/jwt_private.pem
	@echo ""
	@echo "Keys generated:"
	@echo "  Private: $(DIST)/jwt_private.pem  ← set ZERELIO_JWT_PRIVATE_KEY to this path"
	@echo "  Public:  $(DIST)/jwt_public.pem"

dev: ## Start dev stack with docker compose (auth disabled)
	docker compose up --build

dev-down: ## Stop dev stack
	docker compose down

test: ## Run all tests with race detector
	GOWORK=off CGO_ENABLED=0 go test -buildvcs=false -race -count=1 ./...

lint: ## Run staticcheck
	staticcheck ./...

clean: ## Remove dist/
	rm -rf $(DIST)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'
