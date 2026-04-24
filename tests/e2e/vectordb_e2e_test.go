package e2e

import (
	"testing"
	"time"

	"digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistanceMetrics_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	metrics := []client.DistanceMetric{
		client.DistanceCosine,
		client.DistanceDotProduct,
		client.DistanceEuclidean,
	}

	for _, metric := range metrics {
		cfg := client.CollectionConfig{
			Name:      "test-" + string(metric),
			Dimension: 128,
			Metric:    metric,
		}
		err := cfg.Validate()
		assert.NoError(t, err, "metric %s should be valid", metric)
	}
}

func TestVectorWithMetadata_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	v := client.Vector{
		ID:     "doc-001",
		Values: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		Metadata: map[string]any{
			"title":    "Test Document",
			"category": "testing",
			"score":    float64(0.95),
		},
	}

	err := v.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "doc-001", v.ID)
	assert.Equal(t, 5, len(v.Values))
	assert.Equal(t, "Test Document", v.Metadata["title"])
}

func TestSearchQueryWithFilter_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	query := client.SearchQuery{
		Vector:   []float32{0.1, 0.2, 0.3},
		TopK:     10,
		MinScore: 0.5,
		Filter: map[string]any{
			"category": "testing",
		},
	}

	err := query.Validate()
	assert.NoError(t, err)
	assert.Equal(t, 10, query.TopK)
	assert.Equal(t, float64(0.5), query.MinScore)
	assert.NotNil(t, query.Filter)
}

func TestQdrantClientLifecycle_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	cfg := &qdrant.Config{
		Host:     "localhost",
		HTTPPort: 6333,
		GRPCPort: 6334,
		TLS:      false,
		Timeout:  5 * time.Second,
	}

	c, err := qdrant.NewClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, c)

	// Close should succeed even without connect
	err = c.Close()
	assert.NoError(t, err)
}

func TestQdrantConfigEdgeCases_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	tests := []struct {
		name    string
		config  *qdrant.Config
		wantErr bool
	}{
		{
			name: "zero http_port",
			config: &qdrant.Config{
				Host: "localhost", HTTPPort: 0, GRPCPort: 6334,
				Timeout: time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative grpc_port",
			config: &qdrant.Config{
				Host: "localhost", HTTPPort: 6333, GRPCPort: -1,
				Timeout: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero timeout",
			config: &qdrant.Config{
				Host: "localhost", HTTPPort: 6333, GRPCPort: 6334,
				Timeout: 0,
			},
			wantErr: true,
		},
		{
			name: "port exceeds max",
			config: &qdrant.Config{
				Host: "localhost", HTTPPort: 70000, GRPCPort: 6334,
				Timeout: time.Second,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := qdrant.NewClient(tc.config)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestErrNotConnected_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	assert.NotNil(t, client.ErrNotConnected)
	assert.Contains(t, client.ErrNotConnected.Error(), "not connected")
}

func TestCollectionConfigNoMetric_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")  // SKIP-OK: #short-mode
	}

	// Empty metric should be valid (defaults are handled by backends)
	cfg := client.CollectionConfig{
		Name:      "no-metric-collection",
		Dimension: 256,
		Metric:    "",
	}
	err := cfg.Validate()
	assert.NoError(t, err)
}
