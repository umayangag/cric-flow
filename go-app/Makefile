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

.PHONY: all build clean run-api team-select team-predictor docker-build docker-run fmt fmt-check vet test lint migrate build-migrate init

all: build

build:
	@mkdir -p bin
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(API_BIN) ./cmd/api

build-migrate:
	@mkdir -p bin
	GOFLAGS=$(GOFLAGS) CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(MIGRATE_BIN) ./cmd/tools/migrate

migrate: build-migrate
	POSTGRES_HOST=$(POSTGRES_HOST) POSTGRES_PORT=$(POSTGRES_PORT) POSTGRES_DB=$(POSTGRES_DB) POSTGRES_USER=$(POSTGRES_USER) POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) POSTGRES_SSLMODE=$(POSTGRES_SSLMODE) \
		$(MIGRATE_BIN) -dir=migrations

run-api:
	go run ./cmd/api

# Convenience variables and targets to run CLIs
MATCH ?= 0
FORMAT ?= T20
SEASON ?=
SIZE ?= 11
MIN_BOWLERS ?= 5
REQUIRE_KEEPER ?= 0
FROM_DB ?= 1
POOL ?= ../ml-service/ml/pool.csv
BAT ?= 0
BOWL ?= 0

# Run team selection pipeline
# Examples:
#  make team-select MATCH=1193505 SEASON=2019 FORMAT=T20
#  make team-select MATCH=1193505 SEASON=2019 FROM_DB=0 POOL=../ml-service/ml/pool.csv
team-select:
	@if [ "$(MATCH)" = "0" ] || [ -z "$(SEASON)" ]; then echo "Please set MATCH=<id> and SEASON=<name> (e.g., 2019)"; exit 2; fi
	REQ_FLAG=""; if [ "$(REQUIRE_KEEPER)" = "1" ]; then REQ_FLAG="-require-keeper"; fi; \
	FROM_FLAG="-from-db=true"; if [ "$(FROM_DB)" = "0" ]; then FROM_FLAG="-from-db=false -pool=$(POOL)"; fi; \
	go run ./cmd/team-select -match=$(MATCH) -format=$(FORMAT) -season=$(SEASON) -size=$(SIZE) -min-bowlers=$(MIN_BOWLERS) $$REQ_FLAG $$FROM_FLAG

# Run team predictor (requires ml-service running and ml/pool.csv present)
# Generate the pool first: `make -C ../ml-service export-pool MATCH=<id>`
# Examples:
#  make team-predictor MATCH=1193505 SEASON=2019 FORMAT=T20 BAT=6 BOWL=5
team-predictor:
	@if [ "$(MATCH)" = "0" ] || [ -z "$(SEASON)" ] || [ -z "$(FORMAT)" ]; then echo "Please set MATCH, SEASON, and FORMAT (TEST|ODI|T20I|T20)"; exit 2; fi
	go run ./cmd/team-predictor -match=$(MATCH) -format=$(FORMAT) -season=$(SEASON) -bat=$(BAT) -bowl=$(BOWL)


# Format Go code to align with CI (gofumpt + golines)
fmt:
	@command -v gofumpt >/dev/null 2>&1 || (echo "Install gofumpt: go install mvdan.cc/gofumpt@latest" && exit 1)
	@command -v golines >/dev/null 2>&1 || (echo "Install golines: go install github.com/segmentio/golines@latest" && exit 1)
	gofumpt -w .
	golines -w -m 120 .

# Optional: check-only formatting status similar to CI
fmt-check:
	@command -v gofumpt >/dev/null 2>&1 || (echo "Install gofumpt: go install mvdan.cc/gofumpt@latest" && exit 1)
	@gofumpt -l . | tee /dev/stderr | awk 'NR>0{exit 1} END{exit 0}'

vet:
	go vet ./...

test:
	go test ./...

# Initialize local Go development environment (tools + modules)
init:
	@echo "Setting up Go dev environment..."
	@command -v go >/dev/null 2>&1 || (echo "Go toolchain not found. Please install Go 1.25+ first: https://go.dev/dl/" && exit 1)
	@echo "- Downloading Go module deps"
	go mod download
	@echo "- Installing gofumpt and golines to GOPATH/bin"
	go install mvdan.cc/gofumpt@latest
	go install github.com/segmentio/golines@latest
	@echo "- (Optional) Installing golangci-lint"
	@command -v golangci-lint >/dev/null 2>&1 || (echo "Tip: install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
	@echo "Done. Ensure \"$(shell go env GOPATH)/bin\" is on your PATH. Try: 'make fmt-check' and 'make test'"

docker-build:
	docker build -t umayangag/cric-app-api:latest -f Dockerfile.api .

docker-run:
	docker run --rm -e POSTGRES_HOST=$${POSTGRES_HOST} -e POSTGRES_PORT=$${POSTGRES_PORT} -e POSTGRES_DB=$${POSTGRES_DB} -e POSTGRES_USER=$${POSTGRES_USER} -e POSTGRES_PASSWORD=$${POSTGRES_PASSWORD} -p 8080:8080 --name $(APP_NAME)-api umayangag/cric-app-api:latest

# Lint Go code using golangci-lint (matches CI config)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "Install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run -c ../.golangci.yml


# Import retired players from CSV
# Usage: make import-retired FILE=./data/retired.csv [APPLY=1] [OTHERS_ZERO=1]
import-retired:
	@[ -n "$(FILE)" ] || (echo "Usage: make import-retired FILE=path/to/retired.csv [APPLY=1] [OTHERS_ZERO=1]" && exit 1)
	@echo "Running import-retired with FILE=$(FILE), APPLY=$(APPLY), OTHERS_ZERO=$(OTHERS_ZERO)"
	@APPLY_FLAG=""; \
	if [ "$(APPLY)" = "1" ]; then APPLY_FLAG="--apply"; fi; \
	OZ_FLAG=""; \
	if [ "$(OTHERS_ZERO)" = "1" ]; then OZ_FLAG="--others-zero"; fi; \
	go run ./cmd/import-retired --file "$(FILE)" $$APPLY_FLAG $$OZ_FLAG

# Import wicket keepers from CSV
# Usage: make import-keepers FILE=./data/keepers.csv [APPLY=1] [OTHERS_ZERO=1]
import-keepers:
	@[ -n "$(FILE)" ] || (echo "Usage: make import-keepers FILE=path/to/keepers.csv [APPLY=1] [OTHERS_ZERO=1]" && exit 1)
	@echo "Running import-keepers with FILE=$(FILE), APPLY=$(APPLY), OTHERS_ZERO=$(OTHERS_ZERO)"
	@APPLY_FLAG=""; \
	if [ "$(APPLY)" = "1" ]; then APPLY_FLAG="--apply"; fi; \
	OZ_FLAG=""; \
	if [ "$(OTHERS_ZERO)" = "1" ]; then OZ_FLAG="--others-zero"; fi; \
	go run ./cmd/import-keepers --file "$(FILE)" $$APPLY_FLAG $$OZ_FLAG
