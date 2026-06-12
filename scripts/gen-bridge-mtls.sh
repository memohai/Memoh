#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
go run ./cmd/gen-bridge-mtls "$@"
