package store

import (
	"database/sql"
	"errors"

	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

type Store struct {
	db      *sql.DB
	dbtx    sqlitesqlc.DBTX
	queries *sqlitesqlc.Queries
	inTx    bool
}

func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("sqlite store requires a database handle")
	}
	return NewWithQueries(db, sqlitesqlc.New(db)), nil
}

func NewWithQueries(db *sql.DB, queries *sqlitesqlc.Queries) *Store {
	return newWithQueries(db, db, queries, false)
}

func newWithQueries(db *sql.DB, dbtx sqlitesqlc.DBTX, queries *sqlitesqlc.Queries, inTx bool) *Store {
	return &Store{
		db:      db,
		dbtx:    dbtx,
		queries: queries,
		inTx:    inTx,
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
