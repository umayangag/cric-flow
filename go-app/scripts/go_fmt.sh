#!/usr/bin/env bash
set -euo pipefail

# Format Go code using gofumpt and golines. Self-bootstrap tools if missing.

# shellcheck source=./go_fmt_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/go_fmt_common.sh"

ensure_fmt_tools

gofumpt -w .
golines -w -m "${GOLINES_MAX_LEN}" .
