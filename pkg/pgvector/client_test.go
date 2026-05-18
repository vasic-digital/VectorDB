package pgvector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.vectordb/pkg/client"
)

// mockRow implements Row for testing.
type mockRow struct {
	scanErr  error
	scanVals []any
}

func (r *mockRow) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if len(r.scanVals) != len(dest) {
		return nil
	}
	for i, d := range dest {
		switch dd := d.(type) {
		case *string:
			if s, ok := r.scanVals[i].(string); ok {
				*dd = s
			}
		case *float64:
			if f, ok := r.scanVals[i].(float64); ok {
				*dd = f
			}
		}
	}
	return nil
}

// mockRows implements Rows for testing.
type mockRows struct {
	rows    [][]any
	idx     int
	scanErr error
	iterErr error
	closed  bool
}

func (r *mockRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *mockRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("mockRows: Scan called without preceding Next")
	}
	row := r.rows[r.idx-1]
	if len(row) != len(dest) {
		return fmt.Errorf("mockRows: row width %d != dest width %d", len(row), len(dest))
	}
	for i, d := range dest {
		switch dd := d.(type) {
		case *string:
			if s, ok := row[i].(string); ok {
				*dd = s
			}
		case *float64:
			if f, ok := row[i].(float64); ok {
				*dd = f
			}
		}
	}
	return nil
}

func (r *mockRows) Err() error { return r.iterErr }
func (r *mockRows) Close()     { r.closed = true }

// mockPool implements DBPool for testing.
type mockPool struct {
	pingErr   error
	execErr   error
	execSQL   []string
	execArgs  [][]any
	closed    bool
	queryRow  *mockRow
	queryRows *mockRows
	queryErr  error
	querySQL  []string
	queryArgs [][]any
}

func (p *mockPool) Ping(_ context.Context) error {
	return p.pingErr
}

func (p *mockPool) Exec(_ context.Context, sql string, args ...any) error {
	p.execSQL = append(p.execSQL, sql)
	p.execArgs = append(p.execArgs, args)
	return p.execErr
}

func (p *mockPool) Query(_ context.Context, sql string, args ...any) (Rows, error) {
	p.querySQL = append(p.querySQL, sql)
	p.queryArgs = append(p.queryArgs, args)
	if p.queryErr != nil {
		return nil, p.queryErr
	}
	if p.queryRows != nil {
		return p.queryRows, nil
	}
	return &mockRows{}, nil
}

func (p *mockPool) QueryRow(_ context.Context, sql string, args ...any) Row {
	p.querySQL = append(p.querySQL, sql)
	p.queryArgs = append(p.queryArgs, args)
	if p.queryRow != nil {
		return p.queryRow
	}
	return &mockRow{}
}

func (p *mockPool) Close() {
	p.closed = true
}

func newTestClient(t *testing.T) (*Client, *mockPool) {
	t.Helper()
	config := &Config{
		ConnectionString: "host=localhost dbname=test",
		TablePrefix:      "vdb_",
		Timeout:          5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)

	pool := &mockPool{}
	c.SetPool(pool)

	return c, pool
}

func newConnectedClient(t *testing.T) (*Client, *mockPool) {
	t.Helper()
	c, pool := newTestClient(t)
	err := c.Connect(context.Background())
	require.NoError(t, err)
	return c, pool
}

// =========================================================================
// Config Tests
// =========================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "vectordb_", config.TablePrefix)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Empty(t, config.ConnectionString)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid",
			config: &Config{
				ConnectionString: "host=localhost dbname=test",
			},
			wantErr: false,
		},
		{
			name: "empty connection string",
			config: &Config{
				ConnectionString: "",
			},
			wantErr:   true,
			errSubstr: "connection string is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =========================================================================
// Helper Tests
// =========================================================================

func TestVectorToString(t *testing.T) {
	tests := []struct {
		name     string
		vector   []float32
		expected string
	}{
		{"empty", []float32{}, "[]"},
		{"single", []float32{1.0}, "[1.000000]"},
		{"multiple", []float32{0.1, 0.2, 0.3}, "[0.100000,0.200000,0.300000]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, VectorToString(tt.vector))
		})
	}
}

func TestDistanceOperator(t *testing.T) {
	tests := []struct {
		metric   client.DistanceMetric
		expected string
	}{
		{client.DistanceCosine, "<=>"},
		{client.DistanceDotProduct, "<#>"},
		{client.DistanceEuclidean, "<->"},
		{"", "<=>"},
	}

	for _, tt := range tests {
		t.Run(string(tt.metric), func(t *testing.T) {
			assert.Equal(t, tt.expected, DistanceOperator(tt.metric))
		})
	}
}

// =========================================================================
// Constructor Tests
// =========================================================================

// =========================================================================
// Connection Tests
// =========================================================================

func TestClient_Connect_Success(t *testing.T) {
	c, _ := newTestClient(t)
	err := c.Connect(context.Background())
	require.NoError(t, err)
}

func TestClient_Connect_NoPool(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, err := NewClient(config)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database pool not set")
}

func TestClient_Connect_PingError(t *testing.T) {
	c, pool := newTestClient(t)
	pool.pingErr = fmt.Errorf("connection refused")

	err := c.Connect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ping database")
}

func TestClient_Connect_ExtensionError(t *testing.T) {
	c, pool := newTestClient(t)
	pool.execErr = fmt.Errorf("permission denied")

	err := c.Connect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to enable vector extension")
}

func TestClient_Close(t *testing.T) {
	c, pool := newConnectedClient(t)
	err := c.Close()
	require.NoError(t, err)
	assert.True(t, pool.closed)
}

func TestClient_Close_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	err := c.Close()
	require.NoError(t, err)
}

// =========================================================================
// Upsert Tests
// =========================================================================

func TestClient_Upsert_Success(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execSQL = nil // reset from connect

	err := c.Upsert(context.Background(), "test_coll", []client.Vector{
		{
			ID:       "v1",
			Values:   []float32{0.1, 0.2},
			Metadata: map[string]any{"key": "value"},
		},
	})
	require.NoError(t, err)
	assert.Len(t, pool.execSQL, 1)
	assert.Contains(t, pool.execSQL[0], "vdb_test_coll")
}

func TestClient_Upsert_Empty(t *testing.T) {
	c, _ := newConnectedClient(t)
	err := c.Upsert(context.Background(), "test", []client.Vector{})
	require.NoError(t, err)
}

func TestClient_Upsert_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Upsert_AutoID(t *testing.T) {
	c, _ := newConnectedClient(t)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{Values: []float32{0.1, 0.2}}, // no ID
	})
	require.NoError(t, err)
}

func TestClient_Upsert_ExecError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execErr = fmt.Errorf("table not found")

	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert vector")
}

// =========================================================================
// Delete Tests
// =========================================================================

func TestClient_Delete_Success(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execSQL = nil

	err := c.Delete(context.Background(), "test_coll", []string{"v1", "v2"})
	require.NoError(t, err)
	assert.Len(t, pool.execSQL, 1)
	assert.Contains(t, pool.execSQL[0], "DELETE FROM vdb_test_coll")
	assert.Contains(t, pool.execSQL[0], "'v1'")
}

func TestClient_Delete_Empty(t *testing.T) {
	c, _ := newConnectedClient(t)
	err := c.Delete(context.Background(), "test", []string{})
	require.NoError(t, err)
}

func TestClient_Delete_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	err := c.Delete(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Delete_ExecError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execErr = fmt.Errorf("table not found")

	err := c.Delete(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete vectors")
}

// =========================================================================
// Get Tests
// =========================================================================

func TestClient_Get_EmptyIDs(t *testing.T) {
	c, _ := newConnectedClient(t)
	vectors, err := c.Get(context.Background(), "test", []string{})
	require.NoError(t, err)
	assert.Empty(t, vectors)
}

func TestClient_Get_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Search Tests
// =========================================================================

func TestClient_Search_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Collection Management Tests
// =========================================================================

func TestClient_CreateCollection_Success(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execSQL = nil

	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "embeddings",
		Dimension: 768,
		Metric:    client.DistanceCosine,
	})
	require.NoError(t, err)
	assert.Len(t, pool.execSQL, 1)
	assert.Contains(t, pool.execSQL[0], "CREATE TABLE IF NOT EXISTS vdb_embeddings")
	assert.Contains(t, pool.execSQL[0], "vector(768)")
}

func TestClient_CreateCollection_InvalidConfig(t *testing.T) {
	c, _ := newConnectedClient(t)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid collection config")
}

func TestClient_CreateCollection_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 768,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_CreateCollection_ExecError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execErr = fmt.Errorf("permission denied")

	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 768,
		Metric:    client.DistanceCosine,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create collection table")
}

func TestClient_DeleteCollection_Success(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execSQL = nil

	err := c.DeleteCollection(context.Background(), "test_coll")
	require.NoError(t, err)
	assert.Len(t, pool.execSQL, 1)
	assert.Contains(t, pool.execSQL[0], "DROP TABLE IF EXISTS vdb_test_coll")
}

func TestClient_DeleteCollection_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	err := c.DeleteCollection(context.Background(), "test")
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_DeleteCollection_ExecError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execErr = fmt.Errorf("permission denied")

	err := c.DeleteCollection(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to drop collection table")
}

func TestClient_ListCollections_NotConnected(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "v_",
	}
	c, _ := NewClient(config)
	_, err := c.ListCollections(context.Background())
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Table Name Tests
// =========================================================================

func TestClient_TableName(t *testing.T) {
	config := &Config{
		ConnectionString: "host=localhost",
		TablePrefix:      "vdb_",
	}
	c, _ := NewClient(config)
	assert.Equal(t, "vdb_my_collection", c.tableName("my_collection"))
}

// =========================================================================
// Round-37 §2.3: real SQL dispatch through DBPool
// =========================================================================

// Search wired path — pool returns one row, assert id + score + SQL composition.
func TestClient_Search_Wired_HappyPath(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.querySQL = nil
	pool.queryRows = &mockRows{
		rows: [][]any{
			{"id-a", "[0.1,0.2]", "{}", 0.25},
			{"id-b", "[0.3,0.4]", "{}", 0.75},
		},
	}

	results, err := c.Search(context.Background(), "test_coll", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   2,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "id-a", results[0].ID)
	// cosine distance 0.25 → score 0.75 (1 - distance).
	assert.InDelta(t, 0.75, results[0].Score, 1e-5)
	assert.Equal(t, "id-b", results[1].ID)
	assert.InDelta(t, 0.25, results[1].Score, 1e-5)

	// Assert SQL composition: contains pgvector cosine-distance operator + params.
	require.Len(t, pool.querySQL, 1)
	assert.Contains(t, pool.querySQL[0], "<=>")
	assert.Contains(t, pool.querySQL[0], "vdb_test_coll")
	assert.Contains(t, pool.querySQL[0], "$1")
	assert.Contains(t, pool.querySQL[0], "$2")
	assert.Contains(t, strings.ToUpper(pool.querySQL[0]), "ORDER BY DISTANCE")
	assert.Contains(t, strings.ToUpper(pool.querySQL[0]), "LIMIT $2")
	// Assert iterator closed.
	assert.True(t, pool.queryRows.closed, "Rows.Close() must be called")
}

// Search nil-pool path — preserves round-22 sentinel contract.
func TestClient_Search_NilPool_SentinelPreserved(t *testing.T) {
	c, _ := newConnectedClient(t)
	// Drop pool out from under the client to exercise the defensive branch.
	c.mu.Lock()
	c.pool = nil
	c.mu.Unlock()

	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   5,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPgvectorSearchNotWired)
}

// Search invalid query — surfaces validation error.
func TestClient_Search_InvalidQuery(t *testing.T) {
	c, _ := newConnectedClient(t)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{}, // invalid
		TopK:   5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid search query")
}

// Search pool.Query error — surfaces wrapped error.
func TestClient_Search_QueryError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.queryErr = errors.New("connection lost")
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pgvector search query failed")
	assert.Contains(t, err.Error(), "connection lost")
}

// Search rows.Scan error — surfaces wrapped error.
func TestClient_Search_ScanError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.queryRows = &mockRows{
		rows:    [][]any{{"id-a", "[]", "{}", 0.1}},
		scanErr: errors.New("scan failed"),
	}
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan failed")
}

// Search rows.Err iteration error.
func TestClient_Search_IterErr(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.queryRows = &mockRows{
		rows:    [][]any{},
		iterErr: errors.New("iteration failure"),
	}
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "iteration failure")
}

// Get wired path — pool returns row, assert vector list + SQL composition.
func TestClient_Get_Wired_HappyPath(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.querySQL = nil
	pool.queryRow = &mockRow{
		scanVals: []any{"v1", "[0.1,0.2]", "{}"},
	}
	vectors, err := c.Get(context.Background(), "test_coll", []string{"v1"})
	require.NoError(t, err)
	require.Len(t, vectors, 1)
	assert.Equal(t, "v1", vectors[0].ID)
	require.Len(t, pool.querySQL, 1)
	assert.Contains(t, pool.querySQL[0], "vdb_test_coll")
	assert.Contains(t, pool.querySQL[0], "WHERE id = $1")
}

// Get nil-pool path — preserves round-22 sentinel contract.
func TestClient_Get_NilPool_SentinelPreserved(t *testing.T) {
	c, _ := newConnectedClient(t)
	c.mu.Lock()
	c.pool = nil
	c.mu.Unlock()

	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPgvectorGetNotWired)
}

// Get → ErrVectorNotFound when QueryRow scan returns ErrNoRowsInResultSet.
func TestClient_Get_NotFound_Sentinel(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.queryRow = &mockRow{
		scanErr: ErrNoRowsInResultSet,
	}
	_, err := c.Get(context.Background(), "test", []string{"missing-id"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVectorNotFound,
		"expected ErrVectorNotFound when pool QueryRow returns no rows")
	// Sanity: distinct from the not-wired sentinel.
	assert.NotErrorIs(t, err, ErrPgvectorGetNotWired)
}

// Get → wrapped scan error for non-no-rows failures.
func TestClient_Get_ScanError(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.queryRow = &mockRow{
		scanErr: errors.New("type mismatch"),
	}
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type mismatch")
	assert.NotErrorIs(t, err, ErrVectorNotFound)
}

// Paired-mutation guard: assert that the sentinels are distinct identities.
// A regression that aliased ErrVectorNotFound to ErrPgvectorGetNotWired
// would make NotErrorIs assertions in the wired tests above false-positive.
func TestSentinels_DistinctIdentities(t *testing.T) {
	assert.NotSame(t, &ErrVectorNotFound, &ErrPgvectorGetNotWired)
	assert.NotSame(t, &ErrPgvectorSearchNotWired, &ErrPgvectorGetNotWired)
	assert.False(t, errors.Is(ErrVectorNotFound, ErrPgvectorGetNotWired))
	assert.False(t, errors.Is(ErrPgvectorGetNotWired, ErrVectorNotFound))
}

func TestClient_ListCollections_Connected(t *testing.T) {
	c, _ := newConnectedClient(t)
	_, err := c.ListCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "live database connection")
}

func TestClient_Upsert_NilMetadata(t *testing.T) {
	c, pool := newConnectedClient(t)
	pool.execSQL = nil // reset from connect

	err := c.Upsert(context.Background(), "test_coll", []client.Vector{
		{
			ID:       "v1",
			Values:   []float32{0.1, 0.2},
			Metadata: nil, // No metadata
		},
	})
	require.NoError(t, err)
	assert.Len(t, pool.execSQL, 1)
}
