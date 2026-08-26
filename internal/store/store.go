// Package store implements the relational persistence layer: an embedded
// SQLite database with migrations, per-aggregate repositories, and
// transaction boundaries. On process restart the service reopens the same
// database file and recovers open tasks, leases, retries and version chains
// without replaying wall-clock state.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store owns the SQL database handle and exposes transactional boundaries.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the database at path, applies migrations, and seeds
// the reference catalog when empty. The database survives process restarts.
func Open(path string) (*Store, error) {
	dsn := path
	if path == "" || path == ":memory:" {
		dsn = ":memory:"
	} else {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// A single connection keeps SQLite's single-writer semantics deterministic
	// for the concurrent-race acceptance scenarios.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.seedCatalog(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the raw handle for read-only queries outside a transaction.
func (s *Store) DB() *sql.DB { return s.db }

// InTx runs fn inside a write transaction and rolls back on any error. Unique
// indexes and versioned UPDATEs are the concurrency guard, so the plain
// transaction is sufficient and deterministic under SQLite's writer lock.
func (s *Store) InTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DBTX is the common query surface shared by *sql.DB and *sql.Tx so
// repositories run identically inside or outside a transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// nowUnix returns the current unix seconds for created/audit timestamps.
var nowUnix = func() int64 { return time.Now().Unix() }
