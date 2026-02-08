#!/usr/bin/env bash
set -euo pipefail

# Format Go code using gofumpt and golines. Self-bootstrap tools if missing.

GOBIN="$(go env GOPATH)/bin"
export PATH="${GOBIN}:$PATH"

if ! command -v gofumpt >/dev/null 2>&1 || ! command -v golines >/dev/null 2>&1; then
  echo "Installing gofumpt and golines to ${GOBIN}..."
  go install mvdan.cc/gofumpt@latest || { echo "Failed to install gofumpt"; exit 1; }
  go install github.com/segmentio/golines@latest || { echo "Failed to install golines"; exit 1; }
fi

gofumpt -w .
golines -w -m 120 .
