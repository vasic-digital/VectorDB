package benchmark

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/qdrant"
)

func BenchmarkVector_Validate(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	v := client.Vector{
		ID:     "bench-vec",
		Values: make([]float32, 1536),
	}
	for i := range v.Values {
		v.Values[i] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.Validate()
	}
}

func BenchmarkSearchQuery_Validate(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	q := client.SearchQuery{
		Vector: make([]float32, 1536),
		TopK:   10,
	}
	for i := range q.Vector {
		q.Vector[i] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Validate()
	}
}

func BenchmarkCollectionConfig_Validate(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	cfg := client.CollectionConfig{
		Name:      "benchmark-collection",
		Dimension: 768,
		Metric:    client.DistanceCosine,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Validate()
	}
}

func BenchmarkQdrantConfig_GetHTTPURL(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	cfg := &qdrant.Config{
		Host:     "qdrant.example.com",
		HTTPPort: 6333,
		GRPCPort: 6334,
		TLS:      true,
		Timeout:  30 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetHTTPURL()
	}
}

func BenchmarkQdrantConfig_GetGRPCAddress(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	cfg := &qdrant.Config{
		Host:     "qdrant.example.com",
		HTTPPort: 6333,
		GRPCPort: 6334,
		TLS:      false,
		Timeout:  30 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetGRPCAddress()
	}
}

func BenchmarkQdrantClient_NewClient(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	cfg := qdrant.DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, _ := qdrant.NewClient(cfg)
		if c != nil {
			_ = c.Close()
		}
	}
}

func BenchmarkVectorCreation_WithMetadata(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	values := make([]float32, 768)
	for i := range values {
		values[i] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := client.Vector{
			ID:     fmt.Sprintf("vec-%d", i),
			Values: values,
			Metadata: map[string]any{
				"title":    "benchmark document",
				"category": "test",
				"score":    0.95,
			},
		}
		_ = v.Validate()
	}
}

func BenchmarkSearchQuery_WithFilter(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := client.SearchQuery{
			Vector:   vec,
			TopK:     20,
			MinScore: 0.7,
			Filter: map[string]any{
				"category": "test",
				"active":   true,
			},
		}
		_ = q.Validate()
	}
}
