// Package client provides core interfaces and types for vector database operations.
package client

import (
	"context"
	"errors"
	"fmt"

	"digital.vasic.vectordb/pkg/i18n"
)

// translator is the package-scoped Translator used to render the
// validation error keys defined in pkg/i18n/bundles/active.en.yaml.
// Defaults to NoopTranslator (returns key verbatim) per CONST-046 +
// CONST-051(B): the VectorDB submodule is project-not-aware and never
// reaches into a parent project for its catalogue. Consuming projects
// wire a real translator via SetTranslator.
var translator i18n.Translator = i18n.NoopTranslator{}

// SetTranslator rewires the package-scoped translator used by
// Validate methods on Vector, SearchQuery, and CollectionConfig.
// Calling with nil restores the NoopTranslator default. Safe to call
// at process init; not safe to call concurrently with Validate.
func SetTranslator(t i18n.Translator) {
	if t == nil {
		translator = i18n.NoopTranslator{}
		return
	}
	translator = t
}

// Translator returns the currently installed Translator. Exposed for
// call-site tests that need to assert SetTranslator wiring without
// reaching into package internals.
func Translator() i18n.Translator {
	return translator
}

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

// Validate checks that the vector has valid fields. Validation error
// messages flow through the package-scoped i18n.Translator per
// CONST-046 — consuming projects can rewire SetTranslator to fan out
// locale overrides without touching this submodule.
func (v *Vector) Validate() error {
	if len(v.Values) == 0 {
		return errors.New(translator.T("vectordb_validation_vector_values_empty", nil))
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

// Validate checks that the search query has valid fields. Validation
// error messages flow through the package-scoped i18n.Translator per
// CONST-046.
func (q *SearchQuery) Validate() error {
	if len(q.Vector) == 0 {
		return errors.New(translator.T("vectordb_validation_query_vector_empty", nil))
	}
	if q.TopK <= 0 {
		return errors.New(translator.T("vectordb_validation_topk_positive", nil))
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
// Validation error messages flow through the package-scoped
// i18n.Translator per CONST-046.
func (c *CollectionConfig) Validate() error {
	if c.Name == "" {
		return errors.New(translator.T("vectordb_validation_collection_name_required", nil))
	}
	if c.Dimension < 1 {
		return errors.New(translator.T("vectordb_validation_dimension_min", nil))
	}
	validMetrics := map[DistanceMetric]bool{
		DistanceCosine:     true,
		DistanceDotProduct: true,
		DistanceEuclidean:  true,
	}
	if c.Metric != "" && !validMetrics[c.Metric] {
		// Distance metric value is data, not a localised template — it
		// is echoed verbatim alongside the (legacy) English fragment.
		// Round-121 will lift this surface through the translator with
		// a templated key.
		return fmt.Errorf("invalid distance metric: %s", c.Metric)
	}
	return nil
}

// ErrNotConnected is returned when an operation is attempted
// on a client that is not connected. Round-121 will route this sentinel
// through the translator while keeping errors.Is identity stable.
var ErrNotConnected = errors.New("not connected to vector database")
