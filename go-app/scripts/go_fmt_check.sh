#!/usr/bin/env bash
set -euo pipefail

# Check formatting without writing. Runs the same two tools as go_fmt.sh so that
# anything `make fmt` would rewrite fails here instead of drifting silently.

# shellcheck source=./go_fmt_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/go_fmt_common.sh"

ensure_fmt_tools

unformatted="$(
  {
    gofumpt -l .
    golines -l -m "${GOLINES_MAX_LEN}" .
  } | sort -u
)"

if [ -n "${unformatted}" ]; then
  echo "These files are not formatted. Run 'make -C go-app fmt':" >&2
  echo "${unformatted}" >&2
  exit 1
fi
