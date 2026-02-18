#!/bin/bash
# Run only *.up.sql in sort order so we get 0001 -> 0008 without running .down.sql.
# Requires db/migrations mounted at /migrations (see docker-compose postgres volumes).
set -e
for f in $(ls /migrations/*.up.sql 2>/dev/null | sort); do
  psql -v ON_ERROR_STOP=1 -f "$f" -U "$POSTGRES_USER" -d "$POSTGRES_DB"
done
