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
	callCount := 0
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call succeeds for connect
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		// Second call returns bad JSON
		_, _ = w.Write([]byte("bad json"))
	})
	defer server.Close()

	_, err := c.ListCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Additional Coverage Tests
// =========================================================================

func TestClient_Upsert_RequestError(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "server error"}`))
	})
	defer server.Close()

	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1, 0.2}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert vectors")
}

func TestClient_Search_DefaultTopK(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{},
		})
	})
	defer server.Close()

	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   0, // should default to 10
	})
	require.NoError(t, err)
	assert.Equal(t, float64(10), reqData["topK"])
}

func TestClient_Search_WithFilter(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{
				{"id": "v1", "score": 0.95},
			},
		})
	})
	defer server.Close()

	filter := map[string]any{"category": "test"}
	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   5,
		Filter: filter,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, filter, reqData["filter"])
}

func TestClient_Search_RequestError(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "query failed"}`))
	})
	defer server.Close()

	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   5,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query")
}

func TestClient_Delete_RequestError(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "delete failed"}`))
	})
	defer server.Close()

	err := c.Delete(context.Background(), "test", []string{"v1", "v2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete vectors")
}

func TestClient_Get_MultipleIDs(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		// Verify query params include multiple ids
		assert.Contains(t, r.URL.RawQuery, "ids=v1")
		assert.Contains(t, r.URL.RawQuery, "ids=v2")
		assert.Contains(t, r.URL.RawQuery, "&")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"vectors": map[string]any{
				"v1": map[string]any{"id": "v1", "values": []float32{0.1}},
				"v2": map[string]any{"id": "v2", "values": []float32{0.2}},
			},
		})
	})
	defer server.Close()

	vectors, err := c.Get(context.Background(), "test", []string{"v1", "v2"})
	require.NoError(t, err)
	assert.Len(t, vectors, 2)
}

func TestClient_Get_EmptyNamespace(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		// Verify namespace is not included when empty
		assert.NotContains(t, r.URL.RawQuery, "namespace=")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"vectors": map[string]any{
				"v1": map[string]any{"id": "v1", "values": []float32{0.1}},
			},
		})
	})
	defer server.Close()

	vectors, err := c.Get(context.Background(), "", []string{"v1"})
	require.NoError(t, err)
	assert.Len(t, vectors, 1)
}

func TestClient_Get_RequestError(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "fetch failed"}`))
	})
	defer server.Close()

	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch vectors")
}

func TestClient_DeleteCollection_RequestError(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "namespace delete failed"}`))
	})
	defer server.Close()

	err := c.DeleteCollection(context.Background(), "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete namespace")
}

func TestClient_ListCollections_RequestError(t *testing.T) {
	callCount := 0
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "stats failed"}`))
	})
	defer server.Close()

	_, err := c.ListCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list namespaces")
}

func TestClient_DoRequest_HTTPClientError(t *testing.T) {
	// Create a server but close it immediately to simulate connection error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	config := &Config{
		APIKey:    "test-api-key",
		IndexHost: server.URL,
		Timeout:   5 * time.Second,
	}
	server.Close() // Close immediately

	c, err := NewClient(config)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

func TestClient_DoRequest_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	config := &Config{
		APIKey:    "test-api-key",
		IndexHost: server.URL,
		Timeout:   5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = c.Connect(ctx)
	require.Error(t, err)
}

func TestClient_Search_NegativeTopK(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{},
		})
	})
	defer server.Close()

	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   -5, // negative should default to 10
	})
	require.NoError(t, err)
	assert.Equal(t, float64(10), reqData["topK"])
}

func TestClient_Upsert_WithMetadata(t *testing.T) {
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

	metadata := map[string]any{"key": "value", "count": 42}
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1, 0.2}, Metadata: metadata},
	})
	require.NoError(t, err)

	vecs := reqData["vectors"].([]any)
	v := vecs[0].(map[string]any)
	assert.Equal(t, "v1", v["id"])
	assert.NotNil(t, v["metadata"])
}

// =========================================================================
// Interface Implementation Tests
// =========================================================================

func TestClient_ImplementsVectorStore(t *testing.T) {
	var _ client.VectorStore = (*Client)(nil)
}

func TestClient_ImplementsCollectionManager(t *testing.T) {
	var _ client.CollectionManager = (*Client)(nil)
}

// =========================================================================
// Table-Driven Tests for Complete Coverage
// =========================================================================

func TestClient_Operations_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		serverSetup func(w http.ResponseWriter, r *http.Request)
		operation   func(c *Client) error
		wantErr     bool
		errContains string
	}{
		{
			name: "upsert success with mixed IDs",
			serverSetup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/describe_index_stats" {
					_ = json.NewEncoder(w).Encode(map[string]any{})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"upsertedCount": 2})
			},
			operation: func(c *Client) error {
				return c.Upsert(context.Background(), "ns", []client.Vector{
					{ID: "v1", Values: []float32{0.1}},
					{Values: []float32{0.2}}, // auto-generated ID
				})
			},
			wantErr: false,
		},
		{
			name: "search success with all options",
			serverSetup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/describe_index_stats" {
					_ = json.NewEncoder(w).Encode(map[string]any{})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"matches": []map[string]any{
						{"id": "v1", "score": 0.99, "values": []float32{0.1, 0.2}},
					},
				})
			},
			operation: func(c *Client) error {
				_, err := c.Search(context.Background(), "ns", client.SearchQuery{
					Vector:   []float32{0.1, 0.2},
					TopK:     5,
					Filter:   map[string]any{"type": "test"},
					MinScore: 0.5,
				})
				return err
			},
			wantErr: false,
		},
		{
			name: "get with namespace in query string",
			serverSetup: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/describe_index_stats" {
					_ = json.NewEncoder(w).Encode(map[string]any{})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"vectors": map[string]any{
						"v1": map[string]any{"id": "v1", "values": []float32{0.1}},
					},
				})
			},
			operation: func(c *Client) error {
				_, err := c.Get(context.Background(), "my-namespace", []string{"v1", "v2", "v3"})
				return err
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, server := newConnectedClient(t, tt.serverSetup)
			defer server.Close()

			err := tt.operation(c)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_Search_ResultsWithValues(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{
				{
					"id":       "v1",
					"score":    0.95,
					"values":   []float64{0.1, 0.2, 0.3},
					"metadata": map[string]any{"category": "test"},
				},
			},
		})
	})
	defer server.Close()

	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2, 0.3},
		TopK:   10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "v1", results[0].ID)
	assert.Equal(t, float32(0.95), results[0].Score)
	assert.NotNil(t, results[0].Metadata)
}

func TestClient_Get_WithMetadata(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"vectors": map[string]any{
				"v1": map[string]any{
					"id":       "v1",
					"values":   []float64{0.1, 0.2},
					"metadata": map[string]any{"key": "value"},
				},
			},
		})
	})
	defer server.Close()

	vectors, err := c.Get(context.Background(), "test", []string{"v1"})
	require.NoError(t, err)
	assert.Len(t, vectors, 1)
	assert.Equal(t, "v1", vectors[0].ID)
	assert.NotNil(t, vectors[0].Metadata)
}

func TestClient_DeleteCollection_ValidatesRequest(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.DeleteCollection(context.Background(), "my-namespace")
	require.NoError(t, err)
	assert.Equal(t, true, reqData["deleteAll"])
	assert.Equal(t, "my-namespace", reqData["namespace"])
}

func TestClient_Upsert_ValidatesNamespace(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{"upsertedCount": 1})
	})
	defer server.Close()

	err := c.Upsert(context.Background(), "custom-namespace", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.NoError(t, err)
	assert.Equal(t, "custom-namespace", reqData["namespace"])
}

func TestClient_Delete_ValidatesNamespace(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer server.Close()

	err := c.Delete(context.Background(), "delete-namespace", []string{"v1", "v2"})
	require.NoError(t, err)
	assert.Equal(t, "delete-namespace", reqData["namespace"])
	ids := reqData["ids"].([]any)
	assert.Len(t, ids, 2)
}

func TestClient_Search_ValidatesNamespace(t *testing.T) {
	var reqData map[string]any

	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&reqData)
		_ = json.NewEncoder(w).Encode(map[string]any{"matches": []map[string]any{}})
	})
	defer server.Close()

	_, err := c.Search(context.Background(), "search-namespace", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   10,
	})
	require.NoError(t, err)
	assert.Equal(t, "search-namespace", reqData["namespace"])
	assert.Equal(t, true, reqData["includeMetadata"])
	assert.Equal(t, false, reqData["includeValues"])
}

// =========================================================================
// Edge Case Tests for doRequest
// =========================================================================

func TestClient_DoRequest_InvalidURL(t *testing.T) {
	// Test with an invalid URL that causes NewRequestWithContext to fail
	config := &Config{
		APIKey:    "test-api-key",
		IndexHost: "://invalid-url", // Missing scheme causes URL parse error
		Timeout:   5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)

	// Manually set connected to bypass the connect check
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	err = c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.Error(t, err)
}

func TestClient_Search_EmptyMatches(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{},
		})
	})
	defer server.Close()

	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   10,
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestClient_ListCollections_EmptyNamespaces(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"namespaces": map[string]any{},
		})
	})
	defer server.Close()

	names, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestClient_Get_EmptyVectors(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"vectors": map[string]any{},
		})
	})
	defer server.Close()

	vectors, err := c.Get(context.Background(), "test", []string{"v1"})
	require.NoError(t, err)
	assert.Empty(t, vectors)
}

func TestClient_Search_AllResultsFilteredByMinScore(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []map[string]any{
				{"id": "v1", "score": 0.3},
				{"id": "v2", "score": 0.4},
			},
		})
	})
	defer server.Close()

	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector:   []float32{0.1, 0.2},
		TopK:     10,
		MinScore: 0.9, // All results should be filtered
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// =========================================================================
// Additional Tests for doRequest Body Read Error Coverage
// =========================================================================

func TestClient_doRequest_BodyReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set content-length but don't write full body
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	config := &Config{
		APIKey:    "test-api-key",
		IndexHost: server.URL,
		Timeout:   5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)

	// Manually set connected to test doRequest error path
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	_, err = c.ListCollections(context.Background())
	require.Error(t, err)
}

func TestClient_doRequest_MarshalError(t *testing.T) {
	c, server := newConnectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/describe_index_stats" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		t.Error("should not reach server with unmarshalable body")
	})
	defer server.Close()

	// Create a channel which cannot be marshaled to JSON
	ch := make(chan int)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{
			ID:       "v1",
			Values:   []float32{0.1},
			Metadata: map[string]any{"channel": ch}, // channels cannot be JSON marshaled
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal body")
}
