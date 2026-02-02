package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.vectordb/pkg/client"
)

// mockServer creates a mock Qdrant server for testing.
type mockServer struct {
	server *httptest.Server
	config *Config
}

func newMockServer(handler http.HandlerFunc) *mockServer {
	server := httptest.NewServer(handler)

	urlParts := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(urlParts, ":")
	host := parts[0]
	port := 80
	if len(parts) > 1 {
		_, _ = fmt.Sscanf(parts[1], "%d", &port)
	}

	config := &Config{
		Host:     host,
		HTTPPort: port,
		GRPCPort: 6334,
		Timeout:  5 * time.Second,
	}

	return &mockServer{server: server, config: config}
}

func (m *mockServer) close() {
	m.server.Close()
}

func (m *mockServer) newClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(m.config)
	require.NoError(t, err)
	return c
}

func (m *mockServer) newConnectedClient(t *testing.T) *Client {
	t.Helper()
	c := m.newClient(t)
	err := c.Connect(context.Background())
	require.NoError(t, err)
	return c
}

// =========================================================================
// Config Tests
// =========================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "localhost", config.Host)
	assert.Equal(t, 6333, config.HTTPPort)
	assert.Equal(t, 6334, config.GRPCPort)
	assert.False(t, config.TLS)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, client.DistanceCosine, config.DefaultDistance)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*Config)
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:      "empty host",
			modify:    func(c *Config) { c.Host = "" },
			wantErr:   true,
			errSubstr: "host is required",
		},
		{
			name:      "invalid http port zero",
			modify:    func(c *Config) { c.HTTPPort = 0 },
			wantErr:   true,
			errSubstr: "http_port must be between 1 and 65535",
		},
		{
			name:      "invalid http port too high",
			modify:    func(c *Config) { c.HTTPPort = 70000 },
			wantErr:   true,
			errSubstr: "http_port must be between 1 and 65535",
		},
		{
			name:      "invalid grpc port",
			modify:    func(c *Config) { c.GRPCPort = 0 },
			wantErr:   true,
			errSubstr: "grpc_port must be between 1 and 65535",
		},
		{
			name:      "invalid timeout",
			modify:    func(c *Config) { c.Timeout = 0 },
			wantErr:   true,
			errSubstr: "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.modify(config)
			err := config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetHTTPURL(t *testing.T) {
	tests := []struct {
		name     string
		tls      bool
		expected string
	}{
		{
			name:     "http",
			tls:      false,
			expected: "http://localhost:6333",
		},
		{
			name:     "https",
			tls:      true,
			expected: "https://localhost:6333",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.TLS = tt.tls
			assert.Equal(t, tt.expected, config.GetHTTPURL())
		})
	}
}

func TestConfig_GetGRPCAddress(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "localhost:6334", config.GetGRPCAddress())
}

// =========================================================================
// Client Constructor Tests
// =========================================================================

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &Config{
				Host:     "",
				HTTPPort: 6333,
				GRPCPort: 6334,
				Timeout:  time.Second,
			},
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
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	defer ms.close()

	c := ms.newClient(t)
	err := c.Connect(context.Background())
	require.NoError(t, err)
}

func TestClient_Connect_ServerError(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ms.close()

	c := ms.newClient(t)
	err := c.Connect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unhealthy status")
}

func TestClient_Connect_WithAPIKey(t *testing.T) {
	var receivedKey string
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("api-key")
		w.WriteHeader(http.StatusOK)
	})
	defer ms.close()

	ms.config.APIKey = "test-key"
	c := ms.newClient(t)
	_ = c.Connect(context.Background())
	assert.Equal(t, "test-key", receivedKey)
}

func TestClient_Close(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Close()
	require.NoError(t, err)
}

func TestClient_Connect_Failure(t *testing.T) {
	config := DefaultConfig()
	config.HTTPPort = 59999
	config.Timeout = 100 * time.Millisecond

	c, err := NewClient(config)
	require.NoError(t, err)

	err = c.Connect(context.Background())
	require.Error(t, err)
}

// =========================================================================
// Upsert Tests
// =========================================================================

func TestClient_Upsert_Success(t *testing.T) {
	var requestBody map[string]any
	var requestPath string

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		requestPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]string{"status": "completed"},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	vectors := []client.Vector{
		{
			ID:       "v1",
			Values:   []float32{0.1, 0.2, 0.3},
			Metadata: map[string]any{"key": "value"},
		},
	}

	err := c.Upsert(context.Background(), "test-collection", vectors)
	require.NoError(t, err)
	assert.Equal(t, "/collections/test-collection/points", requestPath)
	assert.NotNil(t, requestBody["points"])
}

func TestClient_Upsert_EmptyVectors(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Error("should not make request for empty vectors")
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Upsert(context.Background(), "test", []client.Vector{})
	require.NoError(t, err)
}

func TestClient_Upsert_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Upsert_AutoID(t *testing.T) {
	var requestBody map[string]any

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]string{"status": "completed"},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{Values: []float32{0.1, 0.2}}, // no ID
	})
	require.NoError(t, err)

	points := requestBody["points"].([]any)
	pt := points[0].(map[string]any)
	assert.NotEmpty(t, pt["id"])
}

func TestClient_Upsert_ServerError(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert points")
}

// =========================================================================
// Search Tests
// =========================================================================

func TestClient_Search_Success(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{
				{"id": "v1", "score": 0.95, "payload": map[string]any{"k": "v"}},
				{"id": "v2", "score": 0.85},
			},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2, 0.3},
		TopK:   10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "v1", results[0].ID)
	assert.Equal(t, float32(0.95), results[0].Score)
	assert.Equal(t, "v", results[0].Metadata["k"])
}

func TestClient_Search_WithFilter(t *testing.T) {
	var requestBody map[string]any

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"result": []any{}})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector:   []float32{0.1, 0.2},
		TopK:     5,
		Filter:   map[string]any{"must": []any{}},
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.NotNil(t, requestBody["filter"])
	assert.Equal(t, 0.7, requestBody["score_threshold"])
}

func TestClient_Search_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Search_ServerError(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	require.Error(t, err)
}

func TestClient_Search_InvalidJSON(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
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
	var requestPath string

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]string{"status": "completed"},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Delete(context.Background(), "test", []string{"v1", "v2"})
	require.NoError(t, err)
	assert.Equal(t, "/collections/test/points/delete", requestPath)
}

func TestClient_Delete_EmptyIDs(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Error("should not make request for empty IDs")
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Delete(context.Background(), "test", []string{})
	require.NoError(t, err)
}

func TestClient_Delete_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	err := c.Delete(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Delete_ServerError(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.Delete(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
}

// =========================================================================
// Get Tests
// =========================================================================

func TestClient_Get_Success(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{
				{
					"id":      "v1",
					"vector":  []float32{0.1, 0.2},
					"payload": map[string]any{"k": "val"},
				},
			},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	vectors, err := c.Get(context.Background(), "test", []string{"v1"})
	require.NoError(t, err)
	assert.Len(t, vectors, 1)
	assert.Equal(t, "v1", vectors[0].ID)
	assert.Equal(t, "val", vectors[0].Metadata["k"])
}

func TestClient_Get_EmptyIDs(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Error("should not make request for empty IDs")
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	vectors, err := c.Get(context.Background(), "test", []string{})
	require.NoError(t, err)
	assert.Empty(t, vectors)
}

func TestClient_Get_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_Get_ServerError(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
}

func TestClient_Get_InvalidJSON(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad json"))
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Collection Management Tests
// =========================================================================

func TestClient_CreateCollection_Success(t *testing.T) {
	var requestBody map[string]any
	var requestPath string

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		requestPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"result": true})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test-coll",
		Dimension: 1536,
		Metric:    client.DistanceCosine,
	})
	require.NoError(t, err)
	assert.Equal(t, "/collections/test-coll", requestPath)

	vectors := requestBody["vectors"].(map[string]any)
	assert.Equal(t, float64(1536), vectors["size"])
	assert.Equal(t, "Cosine", vectors["distance"])
}

func TestClient_CreateCollection_DotProduct(t *testing.T) {
	var requestBody map[string]any

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"result": true})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 768,
		Metric:    client.DistanceDotProduct,
	})
	require.NoError(t, err)
	vectors := requestBody["vectors"].(map[string]any)
	assert.Equal(t, "Dot", vectors["distance"])
}

func TestClient_CreateCollection_Euclidean(t *testing.T) {
	var requestBody map[string]any

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"result": true})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 768,
		Metric:    client.DistanceEuclidean,
	})
	require.NoError(t, err)
	vectors := requestBody["vectors"].(map[string]any)
	assert.Equal(t, "Euclid", vectors["distance"])
}

func TestClient_CreateCollection_InvalidConfig(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid collection config")
}

func TestClient_CreateCollection_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test",
		Dimension: 1536,
		Metric:    client.DistanceCosine,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_DeleteCollection_Success(t *testing.T) {
	var requestPath string
	var requestMethod string

	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		requestPath = r.URL.Path
		requestMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"result": true})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	err := c.DeleteCollection(context.Background(), "test-coll")
	require.NoError(t, err)
	assert.Equal(t, "/collections/test-coll", requestPath)
	assert.Equal(t, http.MethodDelete, requestMethod)
}

func TestClient_DeleteCollection_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	err := c.DeleteCollection(context.Background(), "test")
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_ListCollections_Success(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"collections": []map[string]string{
					{"name": "coll1"},
					{"name": "coll2"},
				},
			},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	names, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"coll1", "coll2"}, names)
}

func TestClient_ListCollections_Empty(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"collections": []map[string]string{},
			},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	names, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestClient_ListCollections_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	_, err := c.ListCollections(context.Background())
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_ListCollections_InvalidJSON(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad"))
	})
	defer ms.close()

	c := ms.newConnectedClient(t)
	_, err := c.ListCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Concurrency Test
// =========================================================================

func TestClient_ConcurrentAccess(t *testing.T) {
	ms := newMockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"collections": []any{},
			},
		})
	})
	defer ms.close()

	c := ms.newConnectedClient(t)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = c.ListCollections(context.Background())
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// =========================================================================
// Metric Mapping Tests
// =========================================================================

func TestMetricToQdrantDistance(t *testing.T) {
	tests := []struct {
		metric   client.DistanceMetric
		expected string
	}{
		{client.DistanceCosine, "Cosine"},
		{client.DistanceDotProduct, "Dot"},
		{client.DistanceEuclidean, "Euclid"},
		{"", "Cosine"}, // default
	}

	for _, tt := range tests {
		t.Run(string(tt.metric), func(t *testing.T) {
			assert.Equal(t, tt.expected, metricToQdrantDistance(tt.metric))
		})
	}
}
