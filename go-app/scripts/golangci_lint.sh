#!/usr/bin/env bash
set -euo pipefail

# Ensure golangci-lint v2 is installed (install if missing or if v1 is present) and run with repo config

GOPATH_BIN="$(go env GOPATH)/bin"
GOLANGCI_BIN="$(command -v golangci-lint 2>/dev/null || true)"
if [[ -z "${GOLANGCI_BIN}" ]]; then
  GOLANGCI_BIN="${GOPATH_BIN}/golangci-lint"
fi

# Defaults mirror Makefile variables
GOLANGCI_VERSION="${GOLANGCI_VERSION:-latest}"
GOLANGCI_MODULE="${GOLANGCI_MODULE:-github.com/golangci/golangci-lint/v2/cmd/golangci-lint}"

if [[ -x "${GOLANGCI_BIN}" ]]; then
  ver="$(${GOLANGCI_BIN} version 2>/dev/null | awk '{print $4}')"
  case "${ver}" in
    v2.*) ;;
    *) echo "Detected golangci-lint ${ver} (not v2), installing v2@${GOLANGCI_VERSION}..."
       go install "${GOLANGCI_MODULE}@${GOLANGCI_VERSION}" || { echo "Failed to install golangci-lint v2"; exit 1; };
       ;;
  esac
else
  echo "golangci-lint not found, installing v2@${GOLANGCI_VERSION}..."
  go install "${GOLANGCI_MODULE}@${GOLANGCI_VERSION}" || { echo "Failed to install golangci-lint v2"; exit 1; };
fi

"${GOLANGCI_BIN}" run -c ../.golangci.yml
