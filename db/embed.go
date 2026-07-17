package db

import "embed"

// MigrationsFS contains all SQL migration files embedded at compile time.
//
//go:embed postgres/migrations/*.sql pgvector/migrations/*.sql
var MigrationsFS embed.FS
