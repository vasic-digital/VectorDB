package integration

import (
	"testing"

	"digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVectorValidation_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		vector  client.Vector
		wantErr bool
	}{
		{
			name:    "valid vector",
			vector:  client.Vector{ID: "v1", Values: []float32{0.1, 0.2, 0.3}},
			wantErr: false,
		},
		{
			name:    "empty values",
			vector:  client.Vector{ID: "v2", Values: []float32{}},
			wantErr: true,
		},
		{
			name:    "nil values",
			vector:  client.Vector{ID: "v3", Values: nil},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.vector.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSearchQueryValidation_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		query   client.SearchQuery
		wantErr bool
	}{
		{
			name:    "valid query",
			query:   client.SearchQuery{Vector: []float32{0.1, 0.2}, TopK: 5},
			wantErr: false,
		},
		{
			name:    "empty vector",
			query:   client.SearchQuery{Vector: []float32{}, TopK: 5},
			wantErr: true,
		},
		{
			name:    "zero top_k",
			query:   client.SearchQuery{Vector: []float32{0.1}, TopK: 0},
			wantErr: true,
		},
		{
			name:    "negative top_k",
			query:   client.SearchQuery{Vector: []float32{0.1}, TopK: -1},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.query.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCollectionConfigValidation_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		config  client.CollectionConfig
		wantErr bool
	}{
		{
			name: "valid config cosine",
			config: client.CollectionConfig{
				Name: "test-collection", Dimension: 128,
				Metric: client.DistanceCosine,
			},
			wantErr: false,
		},
		{
			name: "valid config dot product",
			config: client.CollectionConfig{
				Name: "test-dot", Dimension: 256,
				Metric: client.DistanceDotProduct,
			},
			wantErr: false,
		},
		{
			name: "valid config euclidean",
			config: client.CollectionConfig{
				Name: "test-euc", Dimension: 64,
				Metric: client.DistanceEuclidean,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: client.CollectionConfig{
				Name: "", Dimension: 128,
				Metric: client.DistanceCosine,
			},
			wantErr: true,
		},
		{
			name: "zero dimension",
			config: client.CollectionConfig{
				Name: "test", Dimension: 0,
				Metric: client.DistanceCosine,
			},
			wantErr: true,
		},
		{
			name: "invalid metric",
			config: client.CollectionConfig{
				Name: "test", Dimension: 128,
				Metric: "invalid_metric",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQdrantConfigValidation_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	defaultCfg := qdrant.DefaultConfig()
	err := defaultCfg.Validate()
	assert.NoError(t, err, "default config should be valid")

	assert.Equal(t, "localhost", defaultCfg.Host)
	assert.Equal(t, 6333, defaultCfg.HTTPPort)
	assert.Equal(t, 6334, defaultCfg.GRPCPort)
}

func TestQdrantConfigURLGeneration_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &qdrant.Config{
		Host:     "qdrant.example.com",
		HTTPPort: 6333,
		GRPCPort: 6334,
		TLS:      false,
	}

	assert.Equal(t, "http://qdrant.example.com:6333", cfg.GetHTTPURL())
	assert.Equal(t, "qdrant.example.com:6334", cfg.GetGRPCAddress())

	cfg.TLS = true
	assert.Equal(t, "https://qdrant.example.com:6333", cfg.GetHTTPURL())
}

func TestQdrantClientCreation_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Valid config should create client
	c, err := qdrant.NewClient(qdrant.DefaultConfig())
	require.NoError(t, err)
	assert.NotNil(t, c)

	// Nil config should use defaults
	c, err = qdrant.NewClient(nil)
	require.NoError(t, err)
	assert.NotNil(t, c)

	// Invalid config should error
	_, err = qdrant.NewClient(&qdrant.Config{
		Host:     "",
		HTTPPort: 6333,
		GRPCPort: 6334,
	})
	assert.Error(t, err)
}
