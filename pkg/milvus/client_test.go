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
			config:  &Config{Host: "localhost", Port: 19530, DBName: "default"},
			wantErr: false,
		},
		{
			name:      "empty host",
			config:    &Config{Host: "", Port: 19530},
			wantErr:   true,
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
