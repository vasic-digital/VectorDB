// Package qdrant provides a vector store adapter for the Qdrant vector database.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"digital.vasic.vectordb/pkg/client"
)

// Client implements client.VectorStore and client.CollectionManager
// for the Qdrant vector database using its REST API.
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

// NewClient creates a new Qdrant client.
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

// Connect verifies connectivity to Qdrant.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.healthCheck(ctx); err != nil {
		return fmt.Errorf("failed to connect to Qdrant: %w", err)
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

// Upsert inserts or updates vectors in a collection.
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

	points := make([]map[string]any, len(vectors))
	for i, v := range vectors {
		id := v.ID
		if id == "" {
			id = uuid.New().String()
		}
		points[i] = map[string]any{
			"id":      id,
			"vector":  v.Values,
			"payload": v.Metadata,
		}
	}

	reqBody := map[string]any{"points": points}
	path := fmt.Sprintf("/collections/%s/points", collection)
	_, err := c.doRequest(ctx, http.MethodPut, path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to upsert points: %w", err)
	}

	return nil
}

// Search performs vector similarity search.
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

	reqBody := map[string]any{
		"vector":       query.Vector,
		"limit":        query.TopK,
		"with_payload": true,
		"with_vector":  false,
	}

	if query.MinScore > 0 {
		reqBody["score_threshold"] = query.MinScore
	}
	if query.Filter != nil {
		reqBody["filter"] = query.Filter
	}

	path := fmt.Sprintf("/collections/%s/points/search", collection)
	respBody, err := c.doRequest(ctx, http.MethodPost, path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	var response struct {
		Result []struct {
			ID      string         `json:"id"`
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload,omitempty"`
			Vector  []float32      `json:"vector,omitempty"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]client.SearchResult, len(response.Result))
	for i, r := range response.Result {
		results[i] = client.SearchResult{
			ID:       r.ID,
			Score:    r.Score,
			Vector:   r.Vector,
			Metadata: r.Payload,
		}
	}

	return results, nil
}

// Delete removes vectors by IDs.
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

	reqBody := map[string]any{"points": ids}
	path := fmt.Sprintf("/collections/%s/points/delete", collection)
	_, err := c.doRequest(ctx, http.MethodPost, path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to delete points: %w", err)
	}

	return nil
}

// Get retrieves vectors by IDs.
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

	reqBody := map[string]any{
		"ids":          ids,
		"with_payload": true,
		"with_vector":  true,
	}

	path := fmt.Sprintf("/collections/%s/points", collection)
	respBody, err := c.doRequest(ctx, http.MethodPost, path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get points: %w", err)
	}

	var response struct {
		Result []struct {
			ID      string         `json:"id"`
			Vector  []float32      `json:"vector"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	vectors := make([]client.Vector, len(response.Result))
	for i, r := range response.Result {
		vectors[i] = client.Vector{
			ID:       r.ID,
			Values:   r.Vector,
			Metadata: r.Payload,
		}
	}

	return vectors, nil
}

// CreateCollection creates a new vector collection.
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

	distance := metricToQdrantDistance(config.Metric)

	reqBody := map[string]any{
		"vectors": map[string]any{
			"size":     config.Dimension,
			"distance": distance,
		},
	}

	path := fmt.Sprintf("/collections/%s", config.Name)
	_, err := c.doRequest(ctx, http.MethodPut, path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

// DeleteCollection deletes a collection.
func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}

	path := fmt.Sprintf("/collections/%s", name)
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	return nil
}

// ListCollections returns all collection names.
func (c *Client) ListCollections(ctx context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return nil, client.ErrNotConnected
	}

	respBody, err := c.doRequest(ctx, http.MethodGet, "/collections", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	var response struct {
		Result struct {
			Collections []struct {
				Name string `json:"name"`
			} `json:"collections"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	names := make([]string, len(response.Result.Collections))
	for i, col := range response.Result.Collections {
		names[i] = col.Name
	}

	return names, nil
}

func (c *Client) healthCheck(ctx context.Context) error {
	url := c.config.GetHTTPURL()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.config.APIKey != "" {
		req.Header.Set("api-key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) doRequest(
	ctx context.Context,
	method, path string,
	body any,
) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.config.GetHTTPURL(), path)

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
	if c.config.APIKey != "" {
		req.Header.Set("api-key", c.config.APIKey)
	}

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

func metricToQdrantDistance(m client.DistanceMetric) string {
	switch m {
	case client.DistanceDotProduct:
		return "Dot"
	case client.DistanceEuclidean:
		return "Euclid"
	default:
		return "Cosine"
	}
}
