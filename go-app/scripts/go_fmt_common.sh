#!/usr/bin/env bash
# Shared setup for the Go formatting scripts.
#
# Formatting and checking must agree on both the tool set and the line length.
# When they disagree, `make fmt` rewrites code that `make fmt-check` never
# flags, so the tree drifts and only shows up as churn in an unrelated PR.
# That is exactly what happened while fmt-check ran gofumpt alone: golines
# wrapping went unenforced. Keep the agreement here, not in the callers.

# Maximum line length enforced by golines.
GOLINES_MAX_LEN=120

GOBIN="$(go env GOPATH)/bin"
export PATH="${GOBIN}:$PATH"

# ensure_fmt_tools installs gofumpt and golines when either is missing.
ensure_fmt_tools() {
  if ! command -v gofumpt >/dev/null 2>&1 || ! command -v golines >/dev/null 2>&1; then
    echo "Installing gofumpt and golines to ${GOBIN}..."
    go install mvdan.cc/gofumpt@latest || {
      echo "Failed to install gofumpt"
      exit 1
    }
    go install github.com/segmentio/golines@latest || {
      echo "Failed to install golines"
      exit 1
    }
  fi
}
