
# Context Provider Makefile

# Common variables
COV_MIN ?= 80

.PHONY: all fmt fmt-check vet test lint coverage coverage-check init

all: test

# Format Go code
fmt:
	@bash ./scripts/go_fmt.sh

# Check formatting
fmt-check:
	@bash ./scripts/go_fmt_check.sh

# Run go vet
vet:
	go vet ./...

# Run tests
test:
	go test ./...

# Run lint
lint:
	@bash ./scripts/golangci_lint.sh

# Generate coverage
coverage:
	@PKGS="$(COVERAGE_PACKAGES)"; \
	if [ -z "$$PKGS" ]; then PKGS="./..."; fi; \
	go test -covermode=atomic -coverprofile=coverage.out $$PKGS

# Check coverage threshold
coverage-check:
	@test -f coverage.out || (echo "Run 'make coverage' first" && exit 1)
	@total=$$(go tool cover -func=coverage.out | awk '/total:/{print $$3}' | tr -d '%'); \
		min=$(COV_MIN); \
		awk -v t="$$total" -v m="$$min" 'BEGIN{ if ((t+0) < (m+0)) exit 1; else exit 0 }' /dev/null || { echo "Coverage $$total% is below required $$min%"; exit 1; }; \
		echo "Coverage $$total% meets threshold (>= $$min%)."

# Initialize dev environment
init:
	@echo "Setting up Go dev environment..."
	@command -v go >/dev/null 2>&1 || (echo "Go toolchain not found." && exit 1)
	go mod download
	go install mvdan.cc/gofumpt@latest
	go install github.com/segmentio/golines@latest
	@command -v golangci-lint >/dev/null 2>&1 || (echo "Tip: install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
