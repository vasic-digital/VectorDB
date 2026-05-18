// Package pgvector — pgxpool adapter providing the real DBPool
// implementation backed by github.com/jackc/pgx/v5/pgxpool.
//
// §11.4 / CONST-050(A) / round-37 §2.3 — this file is the only spot
// in the package that imports pgx directly. The Client / DBPool
// abstraction in client.go stays pgx-free so unit tests run without
// network access and so non-Postgres backends could provide their
// own DBPool implementation if needed.
//
// CONST-042 — the DSN is sourced by the caller (env var, config
// file, secret manager). NEVER hardcode credentials in this file.
package pgvector

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgvectorConfig is the constructor input for NewPgvectorClient.
//
// DSN must be sourced from environment / secret store — never
// hardcoded. Empty DSN produces a "no pool" client whose Search
// and Get return the round-22 sentinels (back-compat).
type PgvectorConfig struct {
	// DSN is a libpq-format Postgres connection string. Required for
	// a wired client; empty produces a sentinel-only client.
	DSN string
	// TablePrefix overrides the default "vectordb_" prefix.
	TablePrefix string
}

// NewPgvectorClient constructs a pgvector Client.
//
// Behaviour:
//   - cfg.DSN == ""  → pool = nil; Search returns ErrPgvectorSearchNotWired,
//     Get returns ErrPgvectorGetNotWired (round-22 contract preserved).
//   - cfg.DSN != ""  → pgxpool.New is invoked; on failure the error is
//     returned. On success the pool is wrapped in a pgxPoolAdapter
//     satisfying DBPool. The caller must invoke client.Connect(ctx)
//     to ping + ensure the vector extension.
//
// This is the primary entry point for production code per the
// round-37 §2.3 design.
func NewPgvectorClient(ctx context.Context, cfg PgvectorConfig) (*Client, error) {
	prefix := cfg.TablePrefix
	if prefix == "" {
		prefix = "vectordb_"
	}
	if cfg.DSN == "" {
		// Sentinel-only client (round-22 back-compat path).
		c := &Client{
			config: &Config{
				ConnectionString: "",
				TablePrefix:      prefix,
			},
		}
		return c, nil
	}

	innerCfg := &Config{
		ConnectionString: cfg.DSN,
		TablePrefix:      prefix,
	}
	c, err := NewClient(innerCfg)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New failed: %w", err)
	}

	c.SetPool(&pgxPoolAdapter{pool: pool})
	return c, nil
}

// pgxPoolAdapter wraps *pgxpool.Pool to satisfy the DBPool interface.
// All cross-package boundary translation (e.g. pgx.ErrNoRows →
// ErrNoRowsInResultSet) happens here so client.go stays pgx-free.
type pgxPoolAdapter struct {
	pool *pgxpool.Pool
}

func (a *pgxPoolAdapter) Ping(ctx context.Context) error {
	return a.pool.Ping(ctx)
}

func (a *pgxPoolAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := a.pool.Exec(ctx, sql, args...)
	return err
}

func (a *pgxPoolAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRowsAdapter{rows: rows}, nil
}

func (a *pgxPoolAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return &pgxRowAdapter{row: a.pool.QueryRow(ctx, sql, args...)}
}

func (a *pgxPoolAdapter) Close() {
	a.pool.Close()
}

// pgxRowAdapter wraps pgx.Row to satisfy Row and to translate
// pgx.ErrNoRows into ErrNoRowsInResultSet (so client.go can detect
// "row missing" without importing pgx).
type pgxRowAdapter struct {
	row pgx.Row
}

func (r *pgxRowAdapter) Scan(dest ...any) error {
	if err := r.row.Scan(dest...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %v", ErrNoRowsInResultSet, err)
		}
		return err
	}
	return nil
}

// pgxRowsAdapter wraps pgx.Rows to satisfy Rows.
type pgxRowsAdapter struct {
	rows pgx.Rows
}

func (r *pgxRowsAdapter) Next() bool            { return r.rows.Next() }
func (r *pgxRowsAdapter) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *pgxRowsAdapter) Err() error            { return r.rows.Err() }
func (r *pgxRowsAdapter) Close()                { r.rows.Close() }
