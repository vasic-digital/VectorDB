package stress

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVectorValidation_Concurrent_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			values := make([]float32, 128)
			for j := range values {
				values[j] = rand.Float32()
			}
			v := client.Vector{
				ID:       fmt.Sprintf("vec-%d", idx),
				Values:   values,
				Metadata: map[string]any{"index": idx},
			}
			err := v.Validate()
			assert.NoError(t, err,
				"valid vector should pass in goroutine %d", idx)
		}(i)
	}
	wg.Wait()
}

func TestSearchQueryValidation_Concurrent_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			vec := make([]float32, 64)
			for j := range vec {
				vec[j] = rand.Float32()
			}
			q := client.SearchQuery{
				Vector: vec,
				TopK:   10 + idx%50,
			}
			err := q.Validate()
			assert.NoError(t, err,
				"valid query should pass in goroutine %d", idx)
		}(i)
	}
	wg.Wait()
}

func TestCollectionConfigValidation_Concurrent_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	metrics := []client.DistanceMetric{
		client.DistanceCosine,
		client.DistanceDotProduct,
		client.DistanceEuclidean,
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cfg := client.CollectionConfig{
				Name:      fmt.Sprintf("collection-%d", idx),
				Dimension: 64 + idx%512,
				Metric:    metrics[idx%len(metrics)],
			}
			err := cfg.Validate()
			assert.NoError(t, err,
				"valid config should pass in goroutine %d", idx)
		}(i)
	}
	wg.Wait()
}

func TestQdrantClientCreation_Concurrent_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cfg := &qdrant.Config{
				Host:     "localhost",
				HTTPPort: 6333,
				GRPCPort: 6334,
				TLS:      false,
				Timeout:  time.Duration(1+idx%10) * time.Second,
			}
			c, err := qdrant.NewClient(cfg)
			require.NoError(t, err)
			assert.NotNil(t, c)
			_ = c.Close()
		}(i)
	}
	wg.Wait()
}

func TestQdrantConfigGeneration_Concurrent_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cfg := &qdrant.Config{
				Host:     fmt.Sprintf("host-%d.example.com", idx),
				HTTPPort: 6333 + idx%100,
				GRPCPort: 6334 + idx%100,
				TLS:      idx%2 == 0,
				Timeout:  time.Second,
			}
			url := cfg.GetHTTPURL()
			assert.NotEmpty(t, url)
			addr := cfg.GetGRPCAddress()
			assert.NotEmpty(t, addr)
			if idx%2 == 0 {
				assert.Contains(t, url, "https://")
			} else {
				assert.Contains(t, url, "http://")
			}
		}(i)
	}
	wg.Wait()
}

func TestMassVectorCreation_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const vectorCount = 1000
	const dimension = 256

	vectors := make([]client.Vector, vectorCount)
	for i := range vectors {
		values := make([]float32, dimension)
		for j := range values {
			values[j] = rand.Float32()
		}
		vectors[i] = client.Vector{
			ID:     fmt.Sprintf("vec-%d", i),
			Values: values,
			Metadata: map[string]any{
				"batch":    i / 100,
				"category": fmt.Sprintf("cat-%d", i%10),
			},
		}
	}

	// Validate all vectors concurrently
	const goroutines = 50
	var wg sync.WaitGroup
	chunkSize := vectorCount / goroutines
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(start int) {
			defer wg.Done()
			end := start + chunkSize
			if end > vectorCount {
				end = vectorCount
			}
			for j := start; j < end; j++ {
				err := vectors[j].Validate()
				assert.NoError(t, err)
			}
		}(i * chunkSize)
	}
	wg.Wait()
}
