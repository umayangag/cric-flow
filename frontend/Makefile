.PHONY: install dev build test typecheck fmt fmt-check help list

# Frontend (Vite + React) — Fast Make targets with no implicit installs
# Usage: run explicitly `make install` once, then use `make dev`, `make fmt`, etc.

FRONTEND_PORT ?= 5173

# Install dependencies only when node_modules is missing (idempotent)
install:
	@if [ -f package.json ]; then \
		if [ ! -d node_modules ]; then \
			echo "[frontend] Installing dependencies via npm ci..."; \
			npm ci; \
		else \
			echo "[frontend] node_modules present — skip install"; \
		fi; \
	else \
		echo "[frontend] No package.json found"; \
		exit 2; \
	fi

# Do not run installs implicitly — keep fast and predictable
dev:
	@cp -n .env.example .env 2>/dev/null || true
	npm run dev

build:
	npm run build

test:
	npm run test --if-present

typecheck:
	npm run typecheck --if-present

# Formatting: prefer local devDependencies via npx without network installs
fmt:
	@# Prettier write
	npx --no-install prettier . --write
	@# ESLint auto-fix if configured
	npx --no-install eslint . --fix --ext .js,.jsx,.ts,.tsx --ignore-path .gitignore --report-unused-disable-directives --max-warnings=0 --quiet --no-error-on-unmatched-pattern || true

fmt-check:
	@# Prettier check (non-zero exit on diffs)
	npx --no-install prettier . --check
	@# ESLint check-only if configured (fail on warnings)
	npx --no-install eslint . --ext .js,.jsx,.ts,.tsx --ignore-path .gitignore --report-unused-disable-directives --max-warnings=0 --no-error-on-unmatched-pattern

help:
	@echo "\nFrontend — Make targets (grouped)"
	@echo "---------------------------------"
	@echo "[Setup]"
	@echo "  install            Install npm deps if node_modules is missing"
	@echo
	@echo "[Run & Build]"
	@echo "  dev                Start Vite dev server (port $(FRONTEND_PORT))"
	@echo "  build              Production build (dist/)"
	@echo
	@echo "[Quality]"
	@echo "  fmt / fmt-check    Format with Prettier (+ ESLint fix/check if configured)"
	@echo
	@echo "[Tests & Types]"
	@echo "  test               Run tests (if present)"
	@echo "  typecheck          Run type checking (if present)"
	@echo
	@echo "Tip: use 'make list' to see all phony targets."

list:
	@awk '/^\.PHONY:/{for(i=2;i<=NF;i++)print $$i}' $(MAKEFILE_LIST) | sort -u
