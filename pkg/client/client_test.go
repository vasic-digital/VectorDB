package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVector_Validate(t *testing.T) {
	tests := []struct {
		name      string
		vector    Vector
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid vector",
			vector: Vector{
				ID:     "v1",
				Values: []float32{0.1, 0.2, 0.3},
			},
			wantErr: false,
		},
		{
			name: "valid vector with metadata",
			vector: Vector{
				ID:       "v2",
				Values:   []float32{0.1, 0.2},
				Metadata: map[string]any{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "empty values",
			vector: Vector{
				ID:     "v3",
				Values: []float32{},
			},
			wantErr:   true,
			errSubstr: "values must not be empty",
		},
		{
			name: "nil values",
			vector: Vector{
				ID: "v4",
			},
			wantErr:   true,
			errSubstr: "values must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.vector.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSearchQuery_Validate(t *testing.T) {
	tests := []struct {
		name      string
		query     SearchQuery
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid query",
			query: SearchQuery{
				Vector: []float32{0.1, 0.2, 0.3},
				TopK:   10,
			},
			wantErr: false,
		},
		{
			name: "valid query with filter",
			query: SearchQuery{
				Vector:   []float32{0.1, 0.2},
				TopK:     5,
				Filter:   map[string]any{"category": "test"},
				MinScore: 0.5,
			},
			wantErr: false,
		},
		{
			name: "empty vector",
			query: SearchQuery{
				Vector: []float32{},
				TopK:   10,
			},
			wantErr:   true,
			errSubstr: "query vector must not be empty",
		},
		{
			name: "nil vector",
			query: SearchQuery{
				TopK: 10,
			},
			wantErr:   true,
			errSubstr: "query vector must not be empty",
		},
		{
			name: "zero top_k",
			query: SearchQuery{
				Vector: []float32{0.1, 0.2},
				TopK:   0,
			},
			wantErr:   true,
			errSubstr: "top_k must be positive",
		},
		{
			name: "negative top_k",
			query: SearchQuery{
				Vector: []float32{0.1, 0.2},
				TopK:   -1,
			},
			wantErr:   true,
			errSubstr: "top_k must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCollectionConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    CollectionConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid config with cosine",
			config: CollectionConfig{
				Name:      "test-collection",
				Dimension: 1536,
				Metric:    DistanceCosine,
			},
			wantErr: false,
		},
		{
			name: "valid config with dot product",
			config: CollectionConfig{
				Name:      "test-collection",
				Dimension: 768,
				Metric:    DistanceDotProduct,
			},
			wantErr: false,
		},
		{
			name: "valid config with euclidean",
			config: CollectionConfig{
				Name:      "test-collection",
				Dimension: 384,
				Metric:    DistanceEuclidean,
			},
			wantErr: false,
		},
		{
			name: "valid config with empty metric (default)",
			config: CollectionConfig{
				Name:      "test-collection",
				Dimension: 1536,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			config: CollectionConfig{
				Dimension: 1536,
				Metric:    DistanceCosine,
			},
			wantErr:   true,
			errSubstr: "collection name is required",
		},
		{
			name: "zero dimension",
			config: CollectionConfig{
				Name:   "test",
				Metric: DistanceCosine,
			},
			wantErr:   true,
			errSubstr: "dimension must be at least 1",
		},
		{
			name: "negative dimension",
			config: CollectionConfig{
				Name:      "test",
				Dimension: -1,
				Metric:    DistanceCosine,
			},
			wantErr:   true,
			errSubstr: "dimension must be at least 1",
		},
		{
			name: "invalid metric",
			config: CollectionConfig{
				Name:      "test",
				Dimension: 1536,
				Metric:    "invalid",
			},
			wantErr:   true,
			errSubstr: "invalid distance metric",
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

func TestDistanceMetric_Constants(t *testing.T) {
	assert.Equal(t, DistanceMetric("cosine"), DistanceCosine)
	assert.Equal(t, DistanceMetric("dot_product"), DistanceDotProduct)
	assert.Equal(t, DistanceMetric("euclidean"), DistanceEuclidean)
}

func TestSearchResult_Fields(t *testing.T) {
	result := SearchResult{
		ID:       "test-id",
		Score:    0.95,
		Vector:   []float32{0.1, 0.2, 0.3},
		Metadata: map[string]any{"key": "value"},
	}

	assert.Equal(t, "test-id", result.ID)
	assert.Equal(t, float32(0.95), result.Score)
	assert.Len(t, result.Vector, 3)
	assert.Equal(t, "value", result.Metadata["key"])
}

func TestVector_Fields(t *testing.T) {
	v := Vector{
		ID:       "test-id",
		Values:   []float32{0.1, 0.2, 0.3},
		Metadata: map[string]any{"content": "test"},
	}

	assert.Equal(t, "test-id", v.ID)
	assert.Len(t, v.Values, 3)
	assert.Equal(t, "test", v.Metadata["content"])
}

func TestErrNotConnected(t *testing.T) {
	assert.NotNil(t, ErrNotConnected)
	assert.Contains(t, ErrNotConnected.Error(), "not connected")
}
