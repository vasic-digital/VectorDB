// Package client provides core interfaces and types for vector database operations.
package client

import (
	"context"
	"fmt"
)

// VectorStore defines the core operations for a vector database.
type VectorStore interface {
	// Connect establishes a connection to the vector database.
	Connect(ctx context.Context) error

	// Close releases resources and closes the connection.
	Close() error

	// Upsert inserts or updates vectors in the specified collection.
	Upsert(ctx context.Context, collection string, vectors []Vector) error

	// Search performs vector similarity search in the specified collection.
	Search(
		ctx context.Context,
		collection string,
		query SearchQuery,
	) ([]SearchResult, error)

	// Delete removes vectors by IDs from the specified collection.
	Delete(ctx context.Context, collection string, ids []string) error

	// Get retrieves vectors by IDs from the specified collection.
	Get(ctx context.Context, collection string, ids []string) ([]Vector, error)
}

// CollectionManager defines operations for managing vector collections.
type CollectionManager interface {
	// CreateCollection creates a new vector collection with the given config.
	CreateCollection(ctx context.Context, config CollectionConfig) error

	// DeleteCollection removes a collection by name.
	DeleteCollection(ctx context.Context, name string) error

	// ListCollections returns the names of all collections.
	ListCollections(ctx context.Context) ([]string, error)
}

// Vector represents a vector with its ID, values, and optional metadata.
type Vector struct {
	ID       string         `json:"id"`
	Values   []float32      `json:"values"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate checks that the vector has valid fields.
func (v *Vector) Validate() error {
	if len(v.Values) == 0 {
		return fmt.Errorf("vector values must not be empty")
	}
	return nil
}

// SearchResult represents a single result from a similarity search.
type SearchResult struct {
	ID       string         `json:"id"`
	Score    float32        `json:"score"`
	Vector   []float32      `json:"vector,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SearchQuery defines the parameters for a vector similarity search.
type SearchQuery struct {
	Vector   []float32      `json:"vector"`
	TopK     int            `json:"top_k"`
	Filter   map[string]any `json:"filter,omitempty"`
	MinScore float64        `json:"min_score,omitempty"`
}

// Validate checks that the search query has valid fields.
func (q *SearchQuery) Validate() error {
	if len(q.Vector) == 0 {
		return fmt.Errorf("query vector must not be empty")
	}
	if q.TopK <= 0 {
		return fmt.Errorf("top_k must be positive")
	}
	return nil
}

// DistanceMetric represents the distance function used for similarity.
type DistanceMetric string

const (
	// DistanceCosine measures cosine similarity.
	DistanceCosine DistanceMetric = "cosine"
	// DistanceDotProduct measures dot product similarity.
	DistanceDotProduct DistanceMetric = "dot_product"
	// DistanceEuclidean measures Euclidean (L2) distance.
	DistanceEuclidean DistanceMetric = "euclidean"
)

// CollectionConfig defines parameters for creating a collection.
type CollectionConfig struct {
	Name      string         `json:"name"`
	Dimension int            `json:"dimension"`
	Metric    DistanceMetric `json:"metric"`
}

// Validate checks that the collection config has valid fields.
func (c *CollectionConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("collection name is required")
	}
	if c.Dimension < 1 {
		return fmt.Errorf("dimension must be at least 1")
	}
	validMetrics := map[DistanceMetric]bool{
		DistanceCosine:     true,
		DistanceDotProduct: true,
		DistanceEuclidean:  true,
	}
	if c.Metric != "" && !validMetrics[c.Metric] {
		return fmt.Errorf("invalid distance metric: %s", c.Metric)
	}
	return nil
}

// ErrNotConnected is returned when an operation is attempted
// on a client that is not connected.
var ErrNotConnected = fmt.Errorf("not connected to vector database")
