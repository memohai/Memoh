#!/bin/bash

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="${PROJECT_ROOT}/config.toml"
MIGRATIONS_DIR="${PROJECT_ROOT}/db/migrations"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

if [ ! -f "$CONFIG_FILE" ]; then
    echo -e "${RED}Error: Config file not found${NC}"
    exit 1
fi

if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo -e "${RED}Error: Migrations directory not found${NC}"
    exit 1
fi

parse_toml_value() {
    local key=$1
    local section=$2
    grep -A 20 "^\[$section\]" "$CONFIG_FILE" | grep "^$key" | head -1 | sed 's/.*=[ ]*//' | tr -d '"' | tr -d "'"
}

DB_HOST=$(parse_toml_value "host" "postgres")
DB_PORT=$(parse_toml_value "port" "postgres")
DB_USER=$(parse_toml_value "user" "postgres")
DB_PASSWORD=$(parse_toml_value "password" "postgres")
DB_NAME=$(parse_toml_value "database" "postgres")
DB_SSLMODE=$(parse_toml_value "sslmode" "postgres")

if [ -z "$DB_HOST" ] || [ -z "$DB_PORT" ] || [ -z "$DB_USER" ] || [ -z "$DB_NAME" ]; then
    echo -e "${RED}Error: Invalid database configuration${NC}"
    exit 1
fi

DB_SSLMODE=${DB_SSLMODE:-disable}

export PGPASSWORD="$DB_PASSWORD"
PSQL_CMD="psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME"

if ! $PSQL_CMD -c "SELECT 1;" > /dev/null 2>&1; then
    echo -e "${RED}Error: Cannot connect to database${NC}"
    exit 1
fi

for migration_file in "$MIGRATIONS_DIR"/*.up.sql; do
    if [ -f "$migration_file" ]; then
        if ! $PSQL_CMD -f "$migration_file" > /dev/null 2>&1; then
            echo -e "${RED}Error: Migration failed - $(basename "$migration_file")${NC}"
            exit 1
        fi
    fi
done

echo -e "${GREEN}✓ Database migration completed${NC}"

unset PGPASSWORD

