package milvus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.vectordb/pkg/client"
)

func parseServerURL(t *testing.T, url string) (string, int) {
	t.Helper()
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	parts := strings.Split(url, ":")
	require.Len(t, parts, 2)
	var port int
	for i := 0; i < len(parts[1]); i++ {
		if parts[1][i] >= '0' && parts[1][i] <= '9' {
			port = port*10 + int(parts[1][i]-'0')
		} else {
			break
		}
	}
	return parts[0], port
}

func createTestClient(
	t *testing.T,
	server *httptest.Server,
) *Client {
	t.Helper()
	host, port := parseServerURL(t, server.URL)
	config := &Config{
		Host:    host,
		Port:    port,
		DBName:  "default",
		Timeout: 5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)
	return c
}

func createConnectedClient(
	t *testing.T,
	server *httptest.Server,
) *Client {
	t.Helper()
	c := createTestClient(t, server)
	err := c.Connect(context.Background())
	require.NoError(t, err)
	return c
}

func milvusOK(data any) map[string]any {
	return map[string]any{"code": 0, "data": data}
}

// =========================================================================
// Config Tests
// =========================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "localhost", config.Host)
	assert.Equal(t, 19530, config.Port)
	assert.Equal(t, "default", config.DBName)
	assert.Equal(t, 30*time.Second, config.Timeout)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid",
			config:  &Config{Host: "localhost", Port: 19530},
			wantErr: false,
		},
		{
			name:      "empty host",
			config:    &Config{Host: "", Port: 19530},
			wantErr:   true,
			errSubstr: "host is required",
		},
		{
			name:      "invalid port",
			config:    &Config{Host: "localhost", Port: 0},
			wantErr:   true,
			errSubstr: "invalid port",
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

func TestConfig_GetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		secure   bool
		expected string
	}{
		{"http", false, "http://localhost:19530/v2/vectordb"},
		{"https", true, "https://localhost:19530/v2/vectordb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Host:   "localhost",
				Port:   19530,
				Secure: tt.secure,
			}
			assert.Equal(t, tt.expected, config.GetBaseURL())
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
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(milvusOK([]string{"coll1"}))
		},
	))
	defer server.Close()

	c := createTestClient(t, server)
	err := c.Connect(context.Background())
	require.NoError(t, err)
}

func TestClient_Connect_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	defer server.Close()

	c := createTestClient(t, server)
	err := c.Connect(context.Background())
	require.Error(t, err)
}

func TestClient_Connect_WithToken(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	host, port := parseServerURL(t, server.URL)
	config := &Config{
		Host:    host,
		Port:    port,
		DBName:  "default",
		Token:   "my-token",
		Timeout: 5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)
	_ = c.Connect(context.Background())
	assert.Equal(t, "Bearer my-token", receivedAuth)
}

func TestClient_Close(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Close()
	require.NoError(t, err)
}

// =========================================================================
// Upsert Tests
// =========================================================================

func TestClient_Upsert_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			assert.True(t, strings.HasSuffix(r.URL.Path, "/entities/insert"))
			_ = json.NewEncoder(w).Encode(milvusOK(map[string]any{
				"insertCount": 2,
				"insertIds":   []string{"v1", "v2"},
			}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Upsert(context.Background(), "test_coll", []client.Vector{
		{ID: "v1", Values: []float32{0.1, 0.2}},
		{ID: "v2", Values: []float32{0.3, 0.4}},
	})
	require.NoError(t, err)
}

func TestClient_Upsert_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
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
	var reqData map[string]any
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&reqData)
			_ = json.NewEncoder(w).Encode(milvusOK(map[string]any{
				"insertCount": 1,
			}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{Values: []float32{0.1}}, // no ID
	})
	require.NoError(t, err)

	data := reqData["data"].([]any)
	entry := data[0].(map[string]any)
	assert.NotEmpty(t, entry["id"])
}

// =========================================================================
// Search Tests
// =========================================================================

func TestClient_Search_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": [][]map[string]any{
					{
						{"id": "v1", "distance": 0.1},
						{"id": "v2", "distance": 0.2},
					},
				},
			})
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
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
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1},
		TopK:   5,
	})
	require.Error(t, err)
}

// =========================================================================
// Delete Tests
// =========================================================================

func TestClient_Delete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			assert.True(t, strings.HasSuffix(r.URL.Path, "/entities/delete"))
			_ = json.NewEncoder(w).Encode(milvusOK(nil))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Delete(context.Background(), "test", []string{"v1", "v2"})
	require.NoError(t, err)
}

func TestClient_Delete_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Delete(context.Background(), "test", []string{})
	require.NoError(t, err)
}

func TestClient_Delete_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	err := c.Delete(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Get Tests
// =========================================================================

func TestClient_Get_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []map[string]any{
					{"id": "v1", "text": "hello"},
				},
			})
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	vectors, err := c.Get(context.Background(), "test", []string{"v1"})
	require.NoError(t, err)
	assert.Len(t, vectors, 1)
	assert.Equal(t, "v1", vectors[0].ID)
}

func TestClient_Get_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	vectors, err := c.Get(context.Background(), "test", []string{})
	require.NoError(t, err)
	assert.Empty(t, vectors)
}

func TestClient_Get_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Collection Management Tests
// =========================================================================

func TestClient_CreateCollection_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			assert.True(t, strings.HasSuffix(r.URL.Path, "/collections/create"))
			_ = json.NewEncoder(w).Encode(milvusOK(nil))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test_coll",
		Dimension: 768,
		Metric:    client.DistanceCosine,
	})
	require.NoError(t, err)
}

func TestClient_CreateCollection_InvalidConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
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
		Dimension: 768,
	})
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_DeleteCollection_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			assert.True(t, strings.HasSuffix(r.URL.Path, "/collections/drop"))
			_ = json.NewEncoder(w).Encode(milvusOK(nil))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.DeleteCollection(context.Background(), "test_coll")
	require.NoError(t, err)
}

func TestClient_DeleteCollection_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	err := c.DeleteCollection(context.Background(), "test")
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

func TestClient_ListCollections_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			_ = json.NewEncoder(w).Encode(
				milvusOK([]string{"coll1", "coll2"}),
			)
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	names, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"coll1", "coll2"}, names)
}

func TestClient_ListCollections_NotConnected(t *testing.T) {
	c, _ := NewClient(nil)
	_, err := c.ListCollections(context.Background())
	assert.ErrorIs(t, err, client.ErrNotConnected)
}

// =========================================================================
// Metric Mapping Tests
// =========================================================================

func TestMetricToMilvus(t *testing.T) {
	tests := []struct {
		metric   client.DistanceMetric
		expected string
	}{
		{client.DistanceCosine, "COSINE"},
		{client.DistanceDotProduct, "IP"},
		{client.DistanceEuclidean, "L2"},
		{"", "COSINE"},
	}

	for _, tt := range tests {
		t.Run(string(tt.metric), func(t *testing.T) {
			assert.Equal(t, tt.expected, metricToMilvus(tt.metric))
		})
	}
}

// =========================================================================
// API Error Test
// =========================================================================

func TestClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    100,
				"message": "Collection not found",
			})
		},
	))
	defer server.Close()

	c := createTestClient(t, server)
	_, err := c.listCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error 100")
}

// =========================================================================
// Connection Retry and Edge Cases Tests
// =========================================================================

func TestClient_Connect_WithBasicAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	host, port := parseServerURL(t, server.URL)
	config := &Config{
		Host:     host,
		Port:     port,
		DBName:   "default",
		Username: "admin",
		Password: "secret",
		Timeout:  5 * time.Second,
	}
	c, err := NewClient(config)
	require.NoError(t, err)
	err = c.Connect(context.Background())
	require.NoError(t, err)
	assert.Contains(t, receivedAuth, "Basic")
}

func TestClient_Connect_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createTestClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	err := c.Connect(ctx)
	require.Error(t, err)
}

// =========================================================================
// Search with Filters Edge Cases Tests
// =========================================================================

func TestClient_Search_WithFilter(t *testing.T) {
	var receivedFilter string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			var reqData map[string]any
			_ = json.NewDecoder(r.Body).Decode(&reqData)
			if f, ok := reqData["filter"].(string); ok {
				receivedFilter = f
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": [][]map[string]any{
					{{"id": "v1", "distance": 0.1}},
				},
			})
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   10,
		Filter: map[string]any{"filter": "category == 'tech'"},
	})
	require.NoError(t, err)
	assert.Equal(t, "category == 'tech'", receivedFilter)
}

func TestClient_Search_WithMinScore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": [][]map[string]any{
					{
						{"id": "v1", "distance": 0.1}, // score = 0.9
						{"id": "v2", "distance": 0.3}, // score = 0.7
						{"id": "v3", "distance": 0.8}, // score = 0.2 (filtered)
					},
				},
			})
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	results, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector:   []float32{0.1, 0.2},
		TopK:     10,
		MinScore: 0.5, // Only v1 and v2 should pass
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "v1", results[0].ID)
	assert.Equal(t, "v2", results[1].ID)
}

func TestClient_Search_DefaultTopK(t *testing.T) {
	var receivedLimit float64
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			var reqData map[string]any
			_ = json.NewDecoder(r.Body).Decode(&reqData)
			if l, ok := reqData["limit"].(float64); ok {
				receivedLimit = l
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": [][]map[string]any{},
			})
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   0, // Should default to 10
	})
	require.NoError(t, err)
	assert.Equal(t, float64(10), receivedLimit)
}

func TestClient_Search_ParseResponseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("invalid json"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.Search(context.Background(), "test", client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Upsert with Metadata Tests
// =========================================================================

func TestClient_Upsert_WithMetadata(t *testing.T) {
	var reqData map[string]any
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&reqData)
			_ = json.NewEncoder(w).Encode(milvusOK(map[string]any{
				"insertCount": 1,
			}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{
			ID:     "v1",
			Values: []float32{0.1, 0.2},
			Metadata: map[string]any{
				"category": "tech",
				"score":    42,
			},
		},
	})
	require.NoError(t, err)

	data := reqData["data"].([]any)
	entry := data[0].(map[string]any)
	assert.Equal(t, "tech", entry["category"])
	assert.Equal(t, float64(42), entry["score"])
}

func TestClient_Upsert_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to insert entities")
}

// =========================================================================
// Delete Error Tests
// =========================================================================

func TestClient_Delete_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Delete(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete entities")
}

// =========================================================================
// Get with Vector Values Tests
// =========================================================================

func TestClient_Get_WithVectorValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []map[string]any{
					{
						"id":       "v1",
						"vector":   []any{0.1, 0.2, 0.3},
						"category": "tech",
					},
				},
			})
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	vectors, err := c.Get(context.Background(), "test", []string{"v1"})
	require.NoError(t, err)
	require.Len(t, vectors, 1)
	assert.Equal(t, "v1", vectors[0].ID)
	assert.Len(t, vectors[0].Values, 3)
	assert.InDelta(t, 0.1, vectors[0].Values[0], 0.001)
	assert.Equal(t, "tech", vectors[0].Metadata["category"])
}

func TestClient_Get_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get entities")
}

func TestClient_Get_ParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("invalid json"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.Get(context.Background(), "test", []string{"v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// Collection Management Error Tests
// =========================================================================

func TestClient_CreateCollection_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.CreateCollection(context.Background(), client.CollectionConfig{
		Name:      "test_coll",
		Dimension: 768,
		Metric:    client.DistanceCosine,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create collection")
}

func TestClient_DeleteCollection_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.DeleteCollection(context.Background(), "test_coll")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to drop collection")
}

func TestClient_ListCollections_ParseError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				// First call (Connect)
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			// Second call (ListCollections) - return invalid JSON
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("invalid json"))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.ListCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

// =========================================================================
// doRequest Edge Cases Tests
// =========================================================================

func TestClient_doRequest_ReadBodyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// Set content length but don't write full body to trigger read error
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte("short"))
			// Connection will be closed, causing read error
		},
	))
	defer server.Close()

	c := createTestClient(t, server)
	// This should fail due to body read error
	err := c.Connect(context.Background())
	require.Error(t, err)
}

func TestConfig_NegativePort(t *testing.T) {
	config := &Config{Host: "localhost", Port: -1}
	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port")
}

// =========================================================================
// Additional doRequest Edge Cases Tests for 100% Coverage
// =========================================================================

func TestClient_doRequest_NilBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	// Directly call listCollections which passes nil body internally
	names, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestClient_doRequest_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "bad request"}`))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestClient_ListCollections_ServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				// First call (Connect) succeeds
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			// Second call (ListCollections) fails
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "server error"}`))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	_, err := c.ListCollections(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list collections")
}

func TestClient_doRequest_CreateRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)
	// Modify config to have invalid URL with control characters
	c.config.Host = "invalid\x00host"

	err := c.Upsert(context.Background(), "test", []client.Vector{
		{ID: "v1", Values: []float32{0.1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestClient_doRequest_MarshalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/collections/list") {
				_ = json.NewEncoder(w).Encode(milvusOK([]string{}))
				return
			}
			t.Error("should not reach server with unmarshalable body")
		},
	))
	defer server.Close()

	c := createConnectedClient(t, server)

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
