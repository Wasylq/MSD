# MSD — common dev targets.
#
# Run `make help` for the full list.

GO       ?= go
PKGS     := ./...
SMOKE_TIMEOUT ?= 5m
GOLINT   ?= golangci-lint

# Use bash for all recipes (need PIPESTATUS, [[ ]], etc. in the smoke target).
SHELL := /bin/bash
# Stricter shell behaviour for every recipe:
#   -u             : error on unset variables (catches typos like $$pas vs $$pass)
#   -o pipefail    : a failing command in a pipe makes the pipe fail
#   -c             : run argument as a command (required when overriding SHELLFLAGS)
# `-e` is intentionally omitted: the `smoke` target uses `;`-chained commands so
# the summary still prints when tests fail, which `set -e` would abort.
.SHELLFLAGS := -u -o pipefail -c

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the msd binary into ./msd.
	$(GO) build -o msd ./cmd/msd/

.PHONY: test
test: ## Run unit tests with race detector (no integration tag).
	$(GO) test -race -count=1 $(PKGS)

.PHONY: smoke
smoke: ## Run integration smoke tests against live sites. Manual only — never in CI.
	@echo "==> Integration smoke tests (live HTTP, not for CI)"
	@echo "==> Tests with placeholder URLs will SKIP."
	@$(GO) test -tags=integration -timeout=$(SMOKE_TIMEOUT) -v ./site/... 2>&1 | tee /tmp/msd-smoke.log; \
	rc=$${PIPESTATUS[0]}; \
	echo ""; \
	echo "========================================"; \
	echo "  SMOKE TEST SUMMARY"; \
	echo "========================================"; \
	pass=$$(grep -c '^--- PASS' /tmp/msd-smoke.log || true); \
	fails=$$(grep -c '^--- FAIL' /tmp/msd-smoke.log || true); \
	skip=$$(grep -c '^--- SKIP' /tmp/msd-smoke.log || true); \
	echo "  PASS: $$pass  FAIL: $$fails  SKIP: $$skip"; \
	echo ""; \
	if [ "$$fails" -gt 0 ]; then \
		echo "  Failed tests:"; \
		grep '^--- FAIL' /tmp/msd-smoke.log | sed 's/^--- FAIL: /    ✗ /' | sed 's/ (.*//' ; \
		echo ""; \
		echo "  Failed packages:"; \
		grep '^FAIL	' /tmp/msd-smoke.log | sed 's/^FAIL/    ✗/' ; \
		echo ""; \
	fi; \
	echo "========================================"; \
	rm -f /tmp/msd-smoke.log; \
	exit $$rc

.PHONY: smoke-one
smoke-one: ## Run smoke for one site handler. Usage: make smoke-one SITE=pixeldrain
	@if [ -z "$(SITE)" ]; then echo "usage: make smoke-one SITE=<name>"; exit 1; fi
	$(GO) test -tags=integration -timeout=$(SMOKE_TIMEOUT) -v ./site/$(SITE)/...

.PHONY: vet
vet: ## go vet on all packages (including integration-tagged).
	$(GO) vet $(PKGS)
	$(GO) vet -tags=integration $(PKGS)

.PHONY: lint
lint: vet ## Run go vet + golangci-lint.
	$(GOLINT) run --timeout=5m

.PHONY: tidy
tidy: ## go mod tidy.
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove built binary and test artifacts.
	rm -f msd msd.exe coverage.out test-output.txt

.PHONY: docker
docker: ## Build the docker image as msd:dev with version metadata from git.
	docker build \
	  --build-arg GO_VERSION=$$(./scripts/go-version.sh) \
	  --build-arg VERSION=$$(git describe --tags --always --dirty) \
	  --build-arg COMMIT=$$(git rev-parse --short HEAD) \
	  --build-arg DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
	  -t msd:dev .
