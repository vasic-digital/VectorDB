// Package pinecone provides a vector store adapter for the Pinecone
// managed vector database service.
package pinecone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"digital.vasic.vectordb/pkg/client"
)

// Config holds Pinecone configuration.
type Config struct {
	APIKey      string        `json:"api_key"`
	Environment string        `json:"environment"`
	IndexHost   string        `json:"index_host"`
	Timeout     time.Duration `json:"timeout"`
}

// DefaultConfig returns a default Pinecone configuration.
func DefaultConfig() *Config {
	return &Config{
		Timeout: 30 * time.Second,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	if c.IndexHost == "" {
		return fmt.Errorf("index host is required")
	}
	return nil
}

// Client implements client.VectorStore and client.CollectionManager
// for the Pinecone vector database. Pinecone uses namespaces rather
// than collections; collection operations map to namespace operations.
type Client struct {
	config     *Config
	httpClient *http.Client
	mu         sync.RWMutex
	connected  bool
}

// Compile-time interface checks.
var (
	_ client.VectorStore       = (*Client)(nil)
	_ client.CollectionManager = (*Client)(nil)
)

// NewClient creates a new Pinecone client.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		connected: false,
	}, nil
}

// Connect verifies connectivity to Pinecone.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.doRequest(ctx, http.MethodPost, "/describe_index_stats", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Pinecone: %w", err)
	}

	c.connected = true
	return nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	return nil
}

// Upsert inserts or updates vectors. The collection parameter is used
// as the Pinecone namespace.
func (c *Client) Upsert(
	ctx context.Context,
	collection string,
	vectors []client.Vector,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}
	if len(vectors) == 0 {
		return nil
	}

	type pineconeVector struct {
		ID       string         `json:"id"`
		Values   []float32      `json:"values"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}

	pVectors := make([]pineconeVector, len(vectors))
	for i, v := range vectors {
		id := v.ID
		if id == "" {
			id = uuid.New().String()
		}
		pVectors[i] = pineconeVector{
			ID:       id,
			Values:   v.Values,
			Metadata: v.Metadata,
		}
	}

	reqBody := map[string]any{
		"vectors":   pVectors,
		"namespace": collection,
	}

	_, err := c.doRequest(ctx, http.MethodPost, "/vectors/upsert", reqBody)
	if err != nil {
		return fmt.Errorf("failed to upsert vectors: %w", err)
	}

	return nil
}

// Search performs vector similarity search. The collection parameter
// is used as the Pinecone namespace.
func (c *Client) Search(
	ctx context.Context,
	collection string,
	query client.SearchQuery,
) ([]client.SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, client.ErrNotConnected
	}

	topK := query.TopK
	if topK <= 0 {
		topK = 10
	}

	reqBody := map[string]any{
		"vector":          query.Vector,
		"topK":            topK,
		"namespace":       collection,
		"includeMetadata": true,
		"includeValues":   false,
	}

	if query.Filter != nil {
		reqBody["filter"] = query.Filter
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/query", reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	var response struct {
		Matches []struct {
			ID       string         `json:"id"`
			Score    float32        `json:"score"`
			Values   []float32      `json:"values,omitempty"`
			Metadata map[string]any `json:"metadata,omitempty"`
		} `json:"matches"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]client.SearchResult, 0, len(response.Matches))
	for _, m := range response.Matches {
		if query.MinScore > 0 && float64(m.Score) < query.MinScore {
			continue
		}
		results = append(results, client.SearchResult{
			ID:       m.ID,
			Score:    m.Score,
			Vector:   m.Values,
			Metadata: m.Metadata,
		})
	}

	return results, nil
}

// Delete removes vectors by IDs. The collection parameter is used
// as the Pinecone namespace.
func (c *Client) Delete(
	ctx context.Context,
	collection string,
	ids []string,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}
	if len(ids) == 0 {
		return nil
	}

	reqBody := map[string]any{
		"ids":       ids,
		"namespace": collection,
	}

	_, err := c.doRequest(ctx, http.MethodPost, "/vectors/delete", reqBody)
	if err != nil {
		return fmt.Errorf("failed to delete vectors: %w", err)
	}

	return nil
}

// Get retrieves vectors by IDs. The collection parameter is used
// as the Pinecone namespace.
func (c *Client) Get(
	ctx context.Context,
	collection string,
	ids []string,
) ([]client.Vector, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, client.ErrNotConnected
	}
	if len(ids) == 0 {
		return []client.Vector{}, nil
	}

	path := "/vectors/fetch?"
	for i, id := range ids {
		if i > 0 {
			path += "&"
		}
		path += "ids=" + id
	}
	if collection != "" {
		path += "&namespace=" + collection
	}

	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch vectors: %w", err)
	}

	var response struct {
		Vectors map[string]struct {
			ID       string         `json:"id"`
			Values   []float32      `json:"values"`
			Metadata map[string]any `json:"metadata,omitempty"`
		} `json:"vectors"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	vectors := make([]client.Vector, 0, len(response.Vectors))
	for _, v := range response.Vectors {
		vectors = append(vectors, client.Vector{
			ID:       v.ID,
			Values:   v.Values,
			Metadata: v.Metadata,
		})
	}

	return vectors, nil
}

// CreateCollection is a no-op for Pinecone since collections (namespaces)
// are created implicitly on first upsert.
func (c *Client) CreateCollection(
	ctx context.Context,
	config client.CollectionConfig,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid collection config: %w", err)
	}
	// Pinecone namespaces are created implicitly.
	return nil
}

// DeleteCollection deletes all vectors in a namespace.
func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}

	reqBody := map[string]any{
		"deleteAll": true,
		"namespace": name,
	}

	_, err := c.doRequest(ctx, http.MethodPost, "/vectors/delete", reqBody)
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	return nil
}

// ListCollections returns all namespaces in the index.
func (c *Client) ListCollections(ctx context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, client.ErrNotConnected
	}

	respBody, err := c.doRequest(
		ctx, http.MethodPost, "/describe_index_stats", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var response struct {
		Namespaces map[string]struct {
			VectorCount int64 `json:"vectorCount"`
		} `json:"namespaces"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	names := make([]string, 0, len(response.Namespaces))
	for ns := range response.Namespaces {
		names = append(names, ns)
	}

	return names, nil
}

func (c *Client) doRequest(
	ctx context.Context,
	method, path string,
	body any,
) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.config.IndexHost, path)

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf(
			"request failed with status %d: %s",
			resp.StatusCode, string(respBody),
		)
	}

	return respBody, nil
}
