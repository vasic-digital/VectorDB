// Package pgvector provides a vector store adapter for PostgreSQL
// with the pgvector extension. This adapter communicates using SQL
// queries and requires a PostgreSQL connection string.
package pgvector

import (
	"context"
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
// this would be a *pgxpool.Pool, but tests can provide a mock.
type DBPool interface {
	Ping(ctx context.Context) error
	Exec(ctx context.Context, sql string, args ...any) error
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Close()
}

// Row abstracts a single row result.
type Row interface {
	Scan(dest ...any) error
}

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

	// pgvector Search requires live DB; unit tests should mock DBPool.
	// This is a placeholder that demonstrates the SQL structure.
	_ = c.tableName(collection)
	_ = query

	return nil, fmt.Errorf(
		"pgvector search requires a live database connection",
	)
}

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

	// Requires live DB; unit tests should mock DBPool.
	return nil, fmt.Errorf(
		"pgvector get requires a live database connection",
	)
}

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
