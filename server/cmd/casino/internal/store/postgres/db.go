// Package postgres persists the casino's balance ledger, round history and NFT
// escrow in the shared telesrv database. It mirrors the Deltament store's
// structure so the two services stay consistent on the same host.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Sentinel store errors, mapped onto HTTP codes at the API boundary.
var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrNoFunds   = errors.New("insufficient funds")
	ErrDuplicate = errors.New("duplicate")
)

// querier abstracts the pool and a transaction behind the same methods.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DB is the casino's Postgres handle.
type DB struct {
	pool *pgxpool.Pool
	q    querier
}

// Open connects to the shared telesrv database.
func Open(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool, q: pool}, nil
}

// Close releases the pool.
func (db *DB) Close() { db.pool.Close() }

// Migrate applies pending casino migrations in filename order.
func (db *DB) Migrate(ctx context.Context) error {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	if _, err := db.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS casino_schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	for _, f := range files {
		var done bool
		if err := db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM casino_schema_migrations WHERE name = $1)`, f.Name()).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			return err
		}
		if _, err := db.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", f.Name(), err)
		}
		if _, err := db.pool.Exec(ctx, `INSERT INTO casino_schema_migrations (name) VALUES ($1)`, f.Name()); err != nil {
			return err
		}
	}
	return nil
}

// InTx runs fn inside a transaction, rolling back on error.
func (db *DB) InTx(ctx context.Context, fn func(*DB) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(&DB{pool: db.pool, q: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// wrap translates pgx errors into the store's sentinel errors.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return ErrDuplicate
		case "23514":
			return ErrNoFunds
		}
	}
	return err
}
