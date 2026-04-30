package store

import (
	"database/sql"
	"errors"

	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

type Store struct {
	db      *sql.DB
	queries *sqlitesqlc.Queries
}

func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("sqlite store requires a database handle")
	}
	return NewWithQueries(db, sqlitesqlc.New(db)), nil
}

func NewWithQueries(db *sql.DB, queries *sqlitesqlc.Queries) *Store {
	return &Store{
		db:      db,
		queries: queries,
	}
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) SQLC() *sqlitesqlc.Queries {
	if s == nil {
		return nil
	}
	return s.queries
}
