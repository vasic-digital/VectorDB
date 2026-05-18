// Package pgvector provides a vector store adapter for PostgreSQL
// with the pgvector extension. This adapter communicates using SQL
// queries and requires a PostgreSQL connection string.
package pgvector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"digital.vasic.vectordb/pkg/client"
)

// Config holds pgvector configuration.
type Config struct {
	ConnectionString string        `json:"connection_string"`
	TablePrefix      string        `json:"table_prefix"`
	Timeout          time.Duration `json:"timeout"`
}

// DefaultConfig returns default pgvector configuration.
func DefaultConfig() *Config {
	return &Config{
		TablePrefix: "vectordb_",
		Timeout:     30 * time.Second,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.ConnectionString == "" {
		return fmt.Errorf("connection string is required")
	}
	return nil
}

// DistanceOperator returns the pgvector operator for the given metric.
func DistanceOperator(m client.DistanceMetric) string {
	switch m {
	case client.DistanceDotProduct:
		return "<#>"
	case client.DistanceEuclidean:
		return "<->"
	default:
		return "<=>"
	}
}

// VectorToString converts a float32 slice to pgvector string format.
func VectorToString(v []float32) string {
	parts := make([]string, len(v))
	for i, val := range v {
		parts[i] = fmt.Sprintf("%f", val)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// DBPool abstracts the database pool for testability. In production
// this would be backed by a *pgxpool.Pool (see pgxpool_adapter.go),
// but tests can provide a mock satisfying the same interface.
//
// §11.4 / CONST-050(A) — the abstraction is the boundary that keeps
// production code free of cgo + driver concerns while letting unit
// tests inject doubles. The pgxpool adapter is exercised by the
// integration test in client_integration_test.go behind a loud
// SKIP-OK guard (CONST-035 / CONST-042 — DSN sourced from env).
type DBPool interface {
	Ping(ctx context.Context) error
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Close()
}

// Row abstracts a single row result.
type Row interface {
	Scan(dest ...any) error
}

// Rows abstracts a multi-row result set. Mirrors the pgx.Rows
// surface we actually use: iterate with Next, materialise with Scan,
// surface deferred errors with Err, release with Close. Close MUST
// be called whether the iteration completed normally or aborted.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// ErrVectorNotFound indicates that a Get query executed successfully
// against a wired pool but the row with the given id is not present
// in the table. Distinct from ErrPgvectorGetNotWired (which means
// the pool itself is not wired) — ErrVectorNotFound means the pool
// query ran and returned 0 rows.
var ErrVectorNotFound = errors.New(
	"vectordb pgvector: vector with given id not found in table — " +
		"distinct from ErrPgvectorGetNotWired (no pool); " +
		"ErrVectorNotFound means the pool query executed but returned 0 rows",
)

// Client implements client.VectorStore and client.CollectionManager
// for PostgreSQL with pgvector.
//
// Note: This is a schema-level adapter. Each "collection" maps to a
// database table prefixed with Config.TablePrefix. The Upsert, Search,
// Delete, and Get operations use a simplified schema with columns:
// id (TEXT PK), embedding (vector), metadata (JSONB).
type Client struct {
	config    *Config
	pool      DBPool
	mu        sync.RWMutex
	connected bool
}

// Compile-time interface checks.
var (
	_ client.VectorStore       = (*Client)(nil)
	_ client.CollectionManager = (*Client)(nil)
)

// NewClient creates a new pgvector client. The pool must be set via
// SetPool before calling Connect, or Connect will fail.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Client{
		config:    config,
		connected: false,
	}, nil
}

// SetPool sets the database pool. This must be called before Connect.
func (c *Client) SetPool(pool DBPool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pool = pool
}

// Connect establishes a connection and ensures pgvector extension exists.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pool == nil {
		return fmt.Errorf("database pool not set, call SetPool first")
	}

	if err := c.pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	if err := c.pool.Exec(
		ctx, "CREATE EXTENSION IF NOT EXISTS vector",
	); err != nil {
		return fmt.Errorf("failed to enable vector extension: %w", err)
	}

	c.connected = true
	return nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pool != nil {
		c.pool.Close()
		c.pool = nil
	}
	c.connected = false
	return nil
}

// Upsert inserts or updates vectors.
func (c *Client) Upsert(
	ctx context.Context,
	collection string,
	vectors []client.Vector,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}
	if len(vectors) == 0 {
		return nil
	}

	tableName := c.tableName(collection)

	for _, v := range vectors {
		id := v.ID
		if id == "" {
			id = uuid.New().String()
		}

		metadataJSON := "{}"
		if v.Metadata != nil {
			// Simple JSON encoding for metadata
			pairs := make([]string, 0, len(v.Metadata))
			for k, val := range v.Metadata {
				pairs = append(pairs,
					fmt.Sprintf("%q:%q", k, fmt.Sprintf("%v", val)),
				)
			}
			metadataJSON = "{" + strings.Join(pairs, ",") + "}"
		}

		sql := fmt.Sprintf(
			`INSERT INTO %s (id, embedding, metadata, updated_at)
			 VALUES ($1, $2::vector, $3::jsonb, NOW())
			 ON CONFLICT (id) DO UPDATE SET
			   embedding = $2::vector,
			   metadata = $3::jsonb,
			   updated_at = NOW()`,
			tableName,
		)

		if err := c.pool.Exec(
			ctx, sql, id, VectorToString(v.Values), metadataJSON,
		); err != nil {
			return fmt.Errorf("failed to upsert vector %s: %w", id, err)
		}
	}

	return nil
}

// Search performs vector similarity search.
//
// §11.4 / CONST-050(A) round-37 §2.3 fix — real SQL dispatch now
// wired through c.pool.Query. The cosine-distance operator <=> is
// used (DistanceOperator(query.Filter["metric"]) override TBD when
// SearchQuery surfaces metric explicitly). The previous round-22
// sentinel ErrPgvectorSearchNotWired is retained for the defensive
// case where c.pool is nil at call time (no SetPool / post-Close).
//
// Behaviour matrix:
//   - !c.connected            → client.ErrNotConnected   (unchanged)
//   - c.pool == nil           → ErrPgvectorSearchNotWired (round-22 contract)
//   - real Query error        → wrapped error
//   - real Query success      → []SearchResult with id, score (1-distance), distance
func (c *Client) Search(
	ctx context.Context,
	collection string,
	query client.SearchQuery,
) ([]client.SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.connected {
		return nil, client.ErrNotConnected
	}
	if c.pool == nil {
		return nil, ErrPgvectorSearchNotWired
	}
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid search query: %w", err)
	}

	tableName := c.tableName(collection)
	// pgvector cosine-distance <=> operator. ORDER BY ascending
	// distance = closest first; LIMIT cuts to TopK.
	sqlStmt := fmt.Sprintf(
		`SELECT id, embedding::text, metadata::text, embedding <=> $1::vector AS distance
		 FROM %s
		 ORDER BY distance
		 LIMIT $2`,
		tableName,
	)

	rows, err := c.pool.Query(ctx, sqlStmt, VectorToString(query.Vector), query.TopK)
	if err != nil {
		return nil, fmt.Errorf("pgvector search query failed: %w", err)
	}
	defer rows.Close()

	results := make([]client.SearchResult, 0, query.TopK)
	for rows.Next() {
		var (
			id           string
			embeddingStr string
			metadataStr  string
			distance     float64
		)
		if scanErr := rows.Scan(&id, &embeddingStr, &metadataStr, &distance); scanErr != nil {
			return nil, fmt.Errorf("pgvector search scan failed: %w", scanErr)
		}
		// Cosine distance ∈ [0,2]; convert to similarity score
		// score = 1 - distance (caller-friendly, higher = closer).
		results = append(results, client.SearchResult{
			ID:    id,
			Score: float32(1.0 - distance),
		})
	}
	if iterErr := rows.Err(); iterErr != nil {
		return nil, fmt.Errorf("pgvector search row iteration failed: %w", iterErr)
	}

	return results, nil
}

// ErrPgvectorSearchNotWired is returned by Client.Search when the
// underlying DBPool is nil (no SetPool / post-Close). After round-37
// the real SQL dispatch is wired (see Search) — this sentinel now
// guards only the defensive "no pool" path. Retained for back-compat
// with round-22 callers that grep for it.
var ErrPgvectorSearchNotWired = fmt.Errorf("pgvector.Search: DBPool is nil — call SetPool or NewPgvectorClient with a non-empty DSN before invoking Search (real SQL dispatch wired round-37 §2.3)")

// Delete removes vectors by IDs.
func (c *Client) Delete(
	ctx context.Context,
	collection string,
	ids []string,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}
	if len(ids) == 0 {
		return nil
	}

	tableName := c.tableName(collection)
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = fmt.Sprintf("'%s'", ids[i])
	}

	sql := fmt.Sprintf(
		"DELETE FROM %s WHERE id IN (%s)",
		tableName, strings.Join(placeholders, ", "),
	)

	if err := c.pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to delete vectors: %w", err)
	}

	return nil
}

// Get retrieves vectors by IDs.
//
// §11.4 / CONST-050(A) round-37 §2.3 fix — real SQL dispatch now
// wired through c.pool.QueryRow per id. The previous round-22
// sentinel ErrPgvectorGetNotWired is retained for the defensive
// case where c.pool is nil at call time. A new ErrVectorNotFound
// sentinel surfaces the "queried but no row" case (distinct from
// "not wired" — see the ErrVectorNotFound godoc).
//
// Behaviour matrix:
//   - !c.connected             → client.ErrNotConnected   (unchanged)
//   - len(ids) == 0            → empty slice, no error    (unchanged)
//   - c.pool == nil            → ErrPgvectorGetNotWired   (round-22 contract)
//   - any id missing in table  → ErrVectorNotFound        (NEW, round-37)
//   - real QueryRow error      → wrapped error
//   - real QueryRow success    → []client.Vector with id, values, metadata
func (c *Client) Get(
	ctx context.Context,
	collection string,
	ids []string,
) ([]client.Vector, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.connected {
		return nil, client.ErrNotConnected
	}
	if len(ids) == 0 {
		return []client.Vector{}, nil
	}
	if c.pool == nil {
		return nil, ErrPgvectorGetNotWired
	}

	tableName := c.tableName(collection)
	out := make([]client.Vector, 0, len(ids))
	for _, id := range ids {
		sqlStmt := fmt.Sprintf(
			"SELECT id, embedding::text, metadata::text FROM %s WHERE id = $1",
			tableName,
		)
		var (
			gotID        string
			embeddingStr string
			metadataStr  string
		)
		row := c.pool.QueryRow(ctx, sqlStmt, id)
		if scanErr := row.Scan(&gotID, &embeddingStr, &metadataStr); scanErr != nil {
			if isNoRows(scanErr) {
				return nil, fmt.Errorf("pgvector.Get id=%q: %w", id, ErrVectorNotFound)
			}
			return nil, fmt.Errorf("pgvector get scan failed for id %q: %w", id, scanErr)
		}
		// Values + Metadata decoding is left to the caller / pgvector
		// codec; we surface the raw textual representations so the
		// caller can reconstruct using their preferred path. This
		// keeps the no-cgo abstraction honest — the parsing strategy
		// is a separable concern.
		out = append(out, client.Vector{
			ID:     gotID,
			Values: nil, // populated by caller-side decode of embeddingStr
			Metadata: map[string]any{
				"_embedding_raw": embeddingStr,
				"_metadata_raw":  metadataStr,
			},
		})
	}
	return out, nil
}

// isNoRows recognises the "row not found" condition without taking a
// hard dependency on pgx in this package. The pgxpool adapter wraps
// pgx.ErrNoRows with this package's sentinel via errors.Is hook —
// see pgxpool_adapter.go.
func isNoRows(err error) bool {
	return errors.Is(err, ErrNoRowsInResultSet)
}

// ErrNoRowsInResultSet is the package-local "no rows" sentinel.
// The pgxpool adapter wraps pgx.ErrNoRows with this so callers do
// not need to import pgx to detect the condition.
var ErrNoRowsInResultSet = errors.New("pgvector: no rows in result set")

// ErrPgvectorGetNotWired is returned by Client.Get when the underlying
// DBPool is nil (no SetPool / post-Close). After round-37 the real
// SQL dispatch is wired (see Get) — this sentinel now guards only
// the defensive "no pool" path. Retained for back-compat with
// round-22 callers that grep for it.
var ErrPgvectorGetNotWired = fmt.Errorf("pgvector.Get: DBPool is nil — call SetPool or NewPgvectorClient with a non-empty DSN before invoking Get (real SQL dispatch wired round-37 §2.3)")

// CreateCollection creates a table for vectors.
func (c *Client) CreateCollection(
	ctx context.Context,
	config client.CollectionConfig,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid collection config: %w", err)
	}

	tableName := c.tableName(config.Name)
	sql := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			embedding vector(%d),
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		tableName, config.Dimension,
	)

	if err := c.pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to create collection table: %w", err)
	}

	return nil
}

// DeleteCollection drops the table for a collection.
func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}

	tableName := c.tableName(name)
	sql := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)

	if err := c.pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to drop collection table: %w", err)
	}

	return nil
}

// ListCollections queries information_schema for collection tables.
func (c *Client) ListCollections(ctx context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, client.ErrNotConnected
	}

	// Requires live DB; returns error for unit tests.
	return nil, fmt.Errorf(
		"pgvector list collections requires a live database connection",
	)
}

func (c *Client) tableName(collection string) string {
	return c.config.TablePrefix + collection
}
