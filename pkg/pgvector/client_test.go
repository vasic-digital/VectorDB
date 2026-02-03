package pgvector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.vectordb/pkg/client"
)

// mockRow implements Row for testing.
type mockRow struct {
	scanErr error
}

func (r *mockRow) Scan(_ ...any) error {
	return r.scanErr
}

// mockPool implements DBPool for testing.
type mockPool struct {
	pingErr  error
	execErr  error
	execSQL  []string
	closed   bool
	queryRow *mockRow
}

func (p *mockPool) Ping(_ context.Context) error {
	return p.pingErr
}

func (p *mockPool) Exec(_ context.Context, sql string, _ ...any) error {
	p.execSQL = append(p.execSQL, sql)
	return p.execErr
}

func (p *mockPool) QueryRow(_ context.Context, sql string, _ ...any) Row {
	p.execSQL = append(p.execSQL, sql)
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

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config fails (empty conn string)",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &Config{
				ConnectionString: "host=localhost",
				TablePrefix:      "v_",
			},
			wantErr: false,
		},
		{
			name:    "empty conn string",
			config:  &Config{ConnectionString: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, c)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, c)
			}
		})
	}
}

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
// Additional Tests for Search, Get, ListCollections Coverage
// =========================================================================

func TestClient_Search_Connected(t *testing.T) {
	c, _ := newConnectedClient(t)
	// Search returns error since it requires live DB
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "live database connection")
}

func TestClient_Get_Connected(t *testing.T) {
	c, _ := newConnectedClient(t)
	// Get returns error since it requires live DB
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "live database connection")
}

func TestClient_ListCollections_Connected(t *testing.T) {
	c, _ := newConnectedClient(t)
	// ListCollections returns error since it requires live DB
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
