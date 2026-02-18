#!/bin/bash
set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

go run "${PROJECT_ROOT}/cmd/agent/main.go" migrate up
