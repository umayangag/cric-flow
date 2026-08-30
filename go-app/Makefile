APP_NAME:=cricinfo
API_BIN:=bin/api
MIGRATE_BIN:=bin/migrate
GOFLAGS:=
LDFLAGS:=-s -w

export POSTGRES_HOST?=localhost
export POSTGRES_PORT?=5432
export POSTGRES_DB?=cricket_data
export POSTGRES_USER?=postgres
export POSTGRES_PASSWORD?=postgres
export POSTGRES_SSLMODE?=disable
# Resource-aware pipelines: limit process memory so concurrency is capped (see docs/config-and-data.md). Override with GOMEMLIMIT=... if needed.
export GOMEMLIMIT?=2GiB

.PHONY: deadcode all build clean run-api team-select team-predictor docker-build docker-run fmt fmt-check vet test lint migrate build-migrate init mocks cricsheet-import

all: build

build:
	@mkdir -p bin
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(API_BIN) ./cmd/api

build-migrate:
	@mkdir -p bin
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(MIGRATE_BIN) ./cmd/migrate

migrate: build-migrate
	POSTGRES_HOST=$(POSTGRES_HOST) POSTGRES_PORT=$(POSTGRES_PORT) POSTGRES_DB=$(POSTGRES_DB) POSTGRES_USER=$(POSTGRES_USER) POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) POSTGRES_SSLMODE=$(POSTGRES_SSLMODE) \
		$(MIGRATE_BIN) -dir=migrations

run-api:
	go run ./cmd/api

# Convenience variables and targets to run CLIs
MATCH ?= 262039498036
FORMAT ?= T20
SEASON ?= 2025
SIZE ?= 11
MIN_BOWLERS ?= 5
REQUIRE_KEEPER ?= 0
BAT ?= 0
BOWL ?= 0

# Run team selection pipeline
# Example:
#  make team-select MATCH=1193505 SEASON=2019 FORMAT=T20
team-select:
	@if [ "$(MATCH)" = "0" ] || [ -z "$(SEASON)" ]; then echo "Please set MATCH=<id> and SEASON=<name> (e.g., 2019)"; exit 2; fi
	REQ_FLAG=""; if [ "$(REQUIRE_KEEPER)" = "1" ]; then REQ_FLAG="-require-keeper"; fi; \
	go run ./cmd/team-select -match=$(MATCH) -format=$(FORMAT) -season=$(SEASON) -size=$(SIZE) -min-bowlers=$(MIN_BOWLERS) $$REQ_FLAG

# Run team predictor (requires ml-service running for predict)
# Examples:
#  make team-predictor MATCH=1193505 SEASON=2019 FORMAT=T20 BAT=6 BOWL=5
team-predictor:
	@if [ "$(MATCH)" = "0" ] || [ -z "$(SEASON)" ] || [ -z "$(FORMAT)" ]; then echo "Please set MATCH, SEASON, and FORMAT (TEST|ODI|T20I|T20)"; exit 2; fi
	go run ./cmd/team-predictor -match=$(MATCH) -format=$(FORMAT) -season=$(SEASON) -bat=$(BAT) -bowl=$(BOWL)

# Import Cricsheet JSON into DB (populates match, match_inning, batting_data, bowling_data).
# Usage: make cricsheet-import [DIR=path/to/json] [PLACEHOLDERS=1] [FAIL_FAST=0]
# PLACEHOLDERS=1 inserts zeroed fielding rows for every player seen.
# FAIL_FAST=0 skips bad files and continues; default is 1 (abort on first error).
cricsheet-import:
	@DIR=$${GO_APP_INPUT_DIR:-../data/go-app/cricsheet}; \
	PLACEHOLDERS_FLAG=""; \
	if [ "$(PLACEHOLDERS)" = "1" ]; then PLACEHOLDERS_FLAG="--placeholders-fielding"; fi; \
	FAIL_FAST_FLAG="--fail-fast=true"; \
	if [ "$(FAIL_FAST)" = "0" ]; then FAIL_FAST_FLAG="--fail-fast=false"; fi; \
	go run ./cmd/cricsheet-importer -in="$$DIR" $$PLACEHOLDERS_FLAG $$FAIL_FAST_FLAG

# Format Go code (gofumpt + golines). Extracted to script for readability.
fmt:
	@bash ./scripts/go_fmt.sh

# Optional: check-only formatting status similar to CI (self-bootstraps gofumpt)
fmt-check:
	@bash ./scripts/go_fmt_check.sh

vet:
	go vet ./...

test:
	go test ./...

# Run tests including DB integration tests (requires a running Postgres configured via POSTGRES_* envs)
.PHONY: test-db
test-db:
	RUN_DB_TESTS=1 go test ./...

# Initialize local Go development environment (tools + modules)
init:
	@echo "Setting up Go dev environment..."
	@command -v go >/dev/null 2>&1 || (echo "Go toolchain not found. Please install Go 1.26+ first: https://go.dev/dl/" && exit 1)
	@echo "- Downloading Go module deps"
	go mod download
	@echo "- Installing gofumpt and golines to GOPATH/bin"
	go install mvdan.cc/gofumpt@latest
	go install github.com/segmentio/golines@latest
	@echo "- Installing golangci-lint $(GOLANGCI_VERSION)"
	go install $(GOLANGCI_MODULE)@$(GOLANGCI_VERSION)
	@echo "Done. Ensure \"$(shell go env GOPATH)/bin\" is on your PATH. Try: 'make fmt-check' and 'make test'"

docker-build:
	docker build -t umayangag/cric-app-api:latest -f Dockerfile.api .

docker-run:
	docker run --rm -e POSTGRES_HOST=$${POSTGRES_HOST} -e POSTGRES_PORT=$${POSTGRES_PORT} -e POSTGRES_DB=$${POSTGRES_DB} -e POSTGRES_USER=$${POSTGRES_USER} -e POSTGRES_PASSWORD=$${POSTGRES_PASSWORD} -p 8080:8080 --name $(APP_NAME)-api umayangag/cric-app-api:latest

# --- Convenience targets for fielding pipeline (Phase 2) ---
# Run only integration tests in go-app/integration
.PHONY: test-int
test-int:
	go test ./integration -v

# Export datasets for all formats to the default output directory
.PHONY: export-dataset
export-dataset:
	OUT=$${GO_APP_OUTPUT_DIR:-../output/go-app}; \
	mkdir -p $$OUT; \
	# Use unified cross-format export to generate *_encoded_all.csv files
	go run ./cmd/export-dataset -unified=1 -out $$OUT

# Lint Go code using golangci-lint (matches CI config)
# Self-bootstrap a pinned major version (v2) if missing or outdated and invoke via absolute path if needed
GOLANGCI_BIN := $(shell command -v golangci-lint 2>/dev/null || echo $$(go env GOPATH)/bin/golangci-lint)
# Use v2 line by default to match .golangci.yml (which requires v2 config schema).
# Pinned to the version .github/workflows/go-app-ci.yml installs, so local lint
# matches CI -- bump both together.
GOLANGCI_VERSION ?= v2.11.1
GOLANGCI_MODULE ?= github.com/golangci/golangci-lint/v2/cmd/golangci-lint

lint:
	$(GOLANGCI_BIN) run -c ../.golangci.yml

# Whole-program reachability. golangci-lint's `unused` only reports unexported
# identifiers within a package, so it never sees an exported repo method that
# nothing calls -- which is most of what docs/CLEANUP_PR_CHECKLIST.md removed.
# Any output here is a failure.
DEADCODE_MODULE ?= golang.org/x/tools/cmd/deadcode
DEADCODE_VERSION ?= v0.38.0
deadcode:
	@out=$$(go run $(DEADCODE_MODULE)@$(DEADCODE_VERSION) -test ./... 2>/dev/null); \
	if [ -n "$$out" ]; then \
	  echo "Unreachable code (delete it, or wire it in):"; \
	  echo "$$out"; \
	  exit 1; \
	fi; \
	echo "deadcode: no unreachable functions."


# Coverage targets
.PHONY: coverage coverage-html coverage-func coverage-check

# Minimum coverage threshold (percentage). Can be overridden: `COV_MIN=63 make coverage-check`
COV_MIN ?= 63

# Exclude from coverage: cmd/* (entrypoints), */mocks (generated), internal/models (data structs),
# internal/db, internal/db/exportqueries (DB-heavy; covered by integration tests with RUN_DB_TESTS=1),
# internal/commands/precomputefeatures, internal/precompute (DB/seqcalc orchestration; integration-covered),
# internal/server (HTTP handlers; covered by API/integration tests)
COVERAGE_EXCLUDE := /mocks|/cmd/|/internal/models$$|/internal/db/exportqueries$$|/internal/db$$|/commands/precomputefeatures$$|/internal/precompute$$|/internal/server$$

# Packages included in coverage. Default: internal libs excluding mocks, cmd, models. Override with COVERAGE_PACKAGES=./... for full.
COVERAGE_PACKAGES ?= $(shell go list ./... | grep -vE '$(COVERAGE_EXCLUDE)' | tr '\n' ' ')

# Generate coverage profile for all packages (or COVERAGE_PACKAGES if set)
coverage:
	go test -covermode=atomic -coverprofile=coverage.out $(COVERAGE_PACKAGES)

# Generate an HTML coverage report (requires a prior coverage run)
coverage-html:
	@test -f coverage.out || (echo "Run 'make coverage' first" && exit 1)
	go tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

# Print the total coverage summary line
coverage-func:
	@test -f coverage.out || (echo "Run 'make coverage' first" && exit 1)
	@go tool cover -func=coverage.out | tail -1

# Fail if total coverage is below COV_MIN (default 63)
coverage-check:
	@test -f coverage.out || (echo "Run 'make coverage' first" && exit 1)
	@total=$$(go tool cover -func=coverage.out | awk '/total:/{print $$3}' | tr -d '%'); \
		min=$(COV_MIN); \
		awk -v t="$$total" -v m="$$min" 'BEGIN{ if ((t+0) < (m+0)) exit 1; else exit 0 }' /dev/null || { echo "Coverage $$total% is below required $$min%"; exit 1; }; \
		echo "Coverage $$total% meets threshold (>= $$min%)."

# Mockery targets for generating interface mocks
.PHONY: mocks mocks-clean

mocks:
	@command -v mockery >/dev/null 2>&1 || (echo "Install mockery: go install github.com/vektra/mockery/v3@v3.6.0" && exit 1)
	mockery --config .mockery.yml

mocks-clean:
	rm -rf internal/**/mocks


# Developer convenience: CI-like coverage runner and help
.PHONY: help coverage-ci list

# Print grouped targets and common variables for the Go app
help:
	@echo "\nGo App — Make targets (grouped)"
	@echo "--------------------------------"
	@echo "[Setup]"
	@echo "  init               Setup Go dev env (modules + tools)"
	@echo "  fmt / fmt-check    Format code / check formatting (gofumpt, golines)"
	@echo "  lint               Run golangci-lint"
	@echo "  deadcode           Fail on functions unreachable from main or tests"
	@echo "  vet                Run go vet"
	@echo "  test               Run unit tests (no DB)"
	@echo "  test-db            Run all tests including DB integration (requires Postgres)"
	@echo "  test-int           Run integration tests in ./integration"
	@echo "  coverage           Run coverage for selected packages"
	@echo "  coverage-check     Enforce coverage threshold (COV_MIN, default 60%%)"
	@echo "  coverage-html      Render HTML coverage report"
	@echo "  coverage-ci        Run coverage + enforce threshold (like CI)"
	@echo
	@echo "[Build & Run]"
	@echo "  build              Build API binary (cmd/api)"
	@echo "  run-api            Run API locally"
	@echo "  migrate            Build + run DB migrator against POSTGRES_* envs"
	@echo
	@echo "[Data & CLIs]"
	@echo "  team-select        Run team selection (uses MATCH, SEASON, FORMAT, SIZE, MIN_BOWLERS, etc.)"
	@echo "  team-predictor     Run team predictor CLI (requires ml-service)"
	@echo "  cricsheet-import   Import Cricsheet JSON into DB (match, match_inning; GO_APP_INPUT_DIR=..., PLACEHOLDERS=1)"
	@echo "  export-dataset     Export unified cross-format datasets to $(GO_APP_OUTPUT_DIR) (writes *_encoded_all.csv)"
	@echo
	@echo "[Docker]"
	@echo "  docker-build       Build API Docker image"
	@echo "  docker-run         Run API Docker container"
	@echo
	@echo "[Mocks/Codegen]"
	@echo "  mocks              Generate interface mocks via mockery"
	@echo "  mocks-clean        Remove generated mocks"
	@echo
	@echo "Variables (with defaults):"
	@echo "  MATCH=$(MATCH)  SEASON=$(SEASON)  FORMAT=$(FORMAT)  SIZE=$(SIZE)  MIN_BOWLERS=$(MIN_BOWLERS)  REQUIRE_KEEPER=$(REQUIRE_KEEPER)"
	@echo "  BAT=$(BAT)  BOWL=$(BOWL)"
	@echo "  COV_MIN=$(COV_MIN)"
	@echo "  COVERAGE_PACKAGES=$(COVERAGE_PACKAGES)"
	@echo "\nTip: use 'make list' to see all phony targets."

# List all .PHONY targets (best-effort)
list:
	@awk '/^\.PHONY:/{for(i=2;i<=NF;i++)print $$i}' $(MAKEFILE_LIST) | sort -u

# CI-like coverage run (respects COV_MIN and COVERAGE_PACKAGES). Useful locally.
coverage-ci:
	@echo "Running coverage for: $(COVERAGE_PACKAGES)"
	$(MAKE) coverage
	@echo "Enforcing coverage >= $(COV_MIN)%"
	$(MAKE) coverage-check
