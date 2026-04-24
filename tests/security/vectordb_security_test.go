package security

import (
	"testing"
	"time"

	"digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVector_EmptyValues_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	v := client.Vector{ID: "test", Values: nil}
	err := v.Validate()
	assert.Error(t, err, "nil values should fail validation")

	v2 := client.Vector{ID: "test", Values: []float32{}}
	err = v2.Validate()
	assert.Error(t, err, "empty values should fail validation")
}

func TestSearchQuery_NilVector_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	q := client.SearchQuery{Vector: nil, TopK: 5}
	err := q.Validate()
	assert.Error(t, err, "nil query vector should fail validation")
}

func TestSearchQuery_NegativeTopK_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	q := client.SearchQuery{
		Vector: []float32{0.1, 0.2},
		TopK:   -10,
	}
	err := q.Validate()
	assert.Error(t, err, "negative TopK should fail validation")
}

func TestCollectionConfig_EmptyName_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	cfg := client.CollectionConfig{Name: "", Dimension: 128}
	err := cfg.Validate()
	assert.Error(t, err, "empty collection name should fail validation")
}

func TestCollectionConfig_NegativeDimension_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	cfg := client.CollectionConfig{
		Name:      "test",
		Dimension: -1,
		Metric:    client.DistanceCosine,
	}
	err := cfg.Validate()
	assert.Error(t, err, "negative dimension should fail validation")
}

func TestCollectionConfig_InvalidMetric_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	cfg := client.CollectionConfig{
		Name:      "test",
		Dimension: 128,
		Metric:    "hamming",
	}
	err := cfg.Validate()
	assert.Error(t, err, "invalid metric should fail validation")
}

func TestQdrantConfig_EmptyHost_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	cfg := &qdrant.Config{
		Host:     "",
		HTTPPort: 6333,
		GRPCPort: 6334,
		Timeout:  time.Second,
	}
	err := cfg.Validate()
	assert.Error(t, err, "empty host should fail validation")
}

func TestQdrantConfig_InvalidPorts_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	tests := []struct {
		name     string
		httpPort int
		grpcPort int
	}{
		{"zero http", 0, 6334},
		{"negative http", -1, 6334},
		{"overflow http", 65536, 6334},
		{"zero grpc", 6333, 0},
		{"negative grpc", 6333, -1},
		{"overflow grpc", 6333, 99999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &qdrant.Config{
				Host:     "localhost",
				HTTPPort: tc.httpPort,
				GRPCPort: tc.grpcPort,
				Timeout:  time.Second,
			}
			err := cfg.Validate()
			assert.Error(t, err)
		})
	}
}

func TestQdrantClient_NilConfig_Security(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")  // SKIP-OK: #short-mode
	}

	// Nil config should not panic, should use defaults
	c, err := qdrant.NewClient(nil)
	require.NoError(t, err)
	assert.NotNil(t, c)
}
