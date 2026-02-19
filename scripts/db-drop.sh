#!/bin/bash
set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "WARNING: This will roll back all database migrations!"
read -p "Type 'yes' to confirm: " confirmation

if [ "$confirmation" != "yes" ]; then
    echo "Cancelled"
    exit 0
fi

go run "${PROJECT_ROOT}/cmd/agent/main.go" migrate down
