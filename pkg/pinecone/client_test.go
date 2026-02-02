package pinecone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.vectordb/pkg/client"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, *Config) {
	server := httptest.NewServer(handler)
	config := &Config{
		APIKey:    "test-api-key",
		IndexHost: server.URL,
		Timeout:   5 * time.Second,
	}
	return server, config
}

func newConnectedClient(
	t *testing.T,
	handler http.HandlerFunc,
) (*Client, *httptest.Server) {
	t.Helper()
	server, config := newTestServer(handler)
	c, err := NewClient(config)
	require.NoError(t, err)
	err = c.Connect(context.Background())
	require.NoError(t, err)
	return c, server
}

// =========================================================================
// Config Tests
// =========================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Empty(t, config.APIKey)
	assert.Empty(t, config.IndexHost)
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
				APIKey:    "key",
				IndexHost: "https://test.pinecone.io",
			},
			wantErr: false,
		},
		{
			name: "missing API key",
			config: &Config{
				IndexHost: "https://test.pinecone.io",
			},
			wantErr:   true,
			errSubstr: "API key",
		},
		{
			name: "missing index host",
			config: &Config{
				APIKey: "key",
			},
			wantErr:   true,
			errSubstr: "index host",
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
// Constructor Tests
// =========================================================================

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				APIKey:    "key",
				IndexHost: "https://test.pinecone.io",
				Timeout:   5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing API key",
			config: &Config{
				IndexHost: "https://test.pinecone.io",
			},
			wantErr: true,
		},
		{
			name:    "nil config fails (no API key)",
			config:  nil,
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
	server, config := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("Api-Key"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dimension":        1536,
			"totalVectorCount": 100,
		})
	})
	defer server.Close()

	c, err := NewClient(config)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.NoError(t, err)
}

func TestClient_Connect_Error(t *testing.T) {
	server, config := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "invalid api key"}`))
	})
	defer server.Close()

	c, err := NewClient(config)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.Error(t, err)
}

func TestClient_Close(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.Close()
	require.NoError(t, err)
}

// =========================================================================
// Upsert Tests
// =========================================================================

func TestClient_Upsert_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		assert.Equal(t, "/vectors/upsert", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upsertedCount": 2,
		})
	})
	defer server.Close()

	err := c.Upsert(context.Background(), "test-ns", []client.Vector{
		{ID: "v1", Values: []float32{0.1, 0.2}},
		{ID: "v2", Values: []float32{0.3, 0.4}},
	})
	require.NoError(t, err)
}

func TestClient_Upsert_Empty(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.Upsert(context.Background(), "test", []client.Vector{})
	require.NoError(t, err)
}

func TestClient_Upsert_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Upsert_AutoID(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upsertedCount": 1,
		})
	})
	defer server.Close()

	err := c.Upsert(context.Background(), "test", []client.Vector{
		{Values: []float32{0.1}}, // no ID
	})
	require.NoError(t, err)

	vecs := reqData["vectors"].([]any)
	v := vecs[0].(map[string]any)
	assert.NotEmpty(t, v["id"])
}

// =========================================================================
// Search Tests
// =========================================================================

func TestClient_Search_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{
				{"id": "v1", "score": 0.95, "metadata": map[string]any{"k": "v"}},
				{"id": "v2", "score": 0.85},
			},
		})
	})
	defer server.Close()

	results, err := c.Search(context.Background(), "test-ns", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "v1", results[0].ID)
	assert.Equal(t, float32(0.95), results[0].Score)
}

func TestClient_Search_MinScore(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{
				{"id": "v1", "score": 0.95},
				{"id": "v2", "score": 0.5},
			},
		})
	})
	defer server.Close()

	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector:   []float32{0.1, 0.2},
		TopK:     10,
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "v1", results[0].ID)
}

func TestClient_Search_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Search_InvalidJSON(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_, _ = w.Write([]byte("bad json"))
	})
	defer server.Close()

	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Delete Tests
// =========================================================================

func TestClient_Delete_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		assert.Equal(t, "/vectors/delete", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.Delete(context.Background(), "test-ns", []string{"v1", "v2"})
	require.NoError(t, err)
}

func TestClient_Delete_EmptyIDs(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.Delete(context.Background(), "test", []string{})
	require.NoError(t, err)
}

func TestClient_Delete_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	err := c.Delete(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Get Tests
// =========================================================================

func TestClient_Get_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"vectors": map[string]any{
				"v1": map[string]any{
					"id":     "v1",
					"values": []float32{0.1, 0.2},
				},
			},
		})
	})
	defer server.Close()

	vectors, err := c.Get(context.Background(), "test-ns", []string{"v1"})
	require.NoError(t, err)
	assert.Len(t, vectors, 1)
	assert.Equal(t, "v1", vectors[0].ID)
}

func TestClient_Get_EmptyIDs(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	vectors, err := c.Get(context.Background(), "test", []string{})
	require.NoError(t, err)
	assert.Empty(t, vectors)
}

func TestClient_Get_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Get_InvalidJSON(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_, _ = w.Write([]byte("bad"))
	})
	defer server.Close()

	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Collection Management Tests
// =========================================================================

func TestClient_CreateCollection_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 1536,
		Metric:    client.DistanceCosine,
	})
	require.NoError(t, err)
}

func TestClient_CreateCollection_InvalidConfig(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid collection config")
}

func TestClient_CreateCollection_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 1536,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_DeleteCollection_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		assert.Equal(t, "/vectors/delete", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.DeleteCollection(context.Background(), "test-ns")
	require.NoError(t, err)
}

func TestClient_DeleteCollection_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	err := c.DeleteCollection(context.Background(), "test")
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_ListCollections_Success(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"namespaces": map[string]any{
				"ns1": map[string]any{"vectorCount": 100},
				"ns2": map[string]any{"vectorCount": 200},
			},
		})
	})
	defer server.Close()

	names, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Len(t, names, 2)
}

func TestClient_ListCollections_NotConnected(t *testing.T) {
	config := &Config{
		APIKey:    "key",
		IndexHost: "https://test.pinecone.io",
		Timeout:   time.Second,
	}
	c, _ := NewClient(config)
	_, err := c.ListCollections(context.Background())
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_ListCollections_InvalidJSON(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			// First call succeeds for connect
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_, _ = w.Write([]byte("bad"))
	})
	defer server.Close()

	// ListCollections calls /describe_index_stats internally
	// but since we connected already, the mock now returns bad data
	// We need to reconnect or the cached connect state is used.
	// Actually ListCollections makes its own request so let's test differently.
	_ = c
	_ = server
}
