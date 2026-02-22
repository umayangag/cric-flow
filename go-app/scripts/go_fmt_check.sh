#!/usr/bin/env bash
set -euo pipefail

# Check formatting using gofumpt; install if missing.

GOBIN="$(go env GOPATH)/bin"
export PATH="${GOBIN}:$PATH"

if ! command -v gofumpt >/dev/null 2>&1; then
  echo "Installing gofumpt to ${GOBIN}..."
  go install mvdan.cc/gofumpt@latest || { echo "Failed to install gofumpt"; exit 1; }
fi

# List files that would be changed; fail if any are listed (awk END runs after exit so we use a flag)
gofumpt -l . | tee /dev/stderr | awk 'NR>0{found=1} END{exit(found?1:0)}'
