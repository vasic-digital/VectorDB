// Package milvus provides a vector store adapter for the Milvus
// vector database using its REST API v2.
package milvus

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

// Config holds Milvus configuration.
type Config struct {
	Host     string        `json:"host"`
	Port     int           `json:"port"`
	Username string        `json:"username,omitempty"`
	Password string        `json:"password,omitempty"`
	DBName   string        `json:"db_name"`
	Secure   bool          `json:"secure"`
	Token    string        `json:"token,omitempty"`
	Timeout  time.Duration `json:"timeout"`
}

// DefaultConfig returns default Milvus configuration.
func DefaultConfig() *Config {
	return &Config{
		Host:    "localhost",
		Port:    19530,
		DBName:  "default",
		Timeout: 30 * time.Second,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port <= 0 {
		return fmt.Errorf("invalid port")
	}
	return nil
}

// GetBaseURL returns the base URL for the Milvus REST API.
func (c *Config) GetBaseURL() string {
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d/v2/vectordb", scheme, c.Host, c.Port)
}

// Client implements client.VectorStore and client.CollectionManager
// for the Milvus vector database.
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

// NewClient creates a new Milvus client.
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

// Connect verifies connectivity to Milvus.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.listCollections(ctx); err != nil {
		return fmt.Errorf("failed to connect to Milvus: %w", err)
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

	data := make([]map[string]any, len(vectors))
	for i, v := range vectors {
		entry := map[string]any{
			"vector": v.Values,
		}
		id := v.ID
		if id == "" {
			id = uuid.New().String()
		}
		entry["id"] = id
		for k, val := range v.Metadata {
			entry[k] = val
		}
		data[i] = entry
	}

	reqBody := map[string]any{
		"dbName":         c.config.DBName,
		"collectionName": collection,
		"data":           data,
	}

	_, err := c.doRequest(ctx, http.MethodPost, "/entities/insert", reqBody)
	if err != nil {
		return fmt.Errorf("failed to insert entities: %w", err)
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

	topK := query.TopK
	if topK <= 0 {
		topK = 10
	}

	reqBody := map[string]any{
		"dbName":         c.config.DBName,
		"collectionName": collection,
		"data":           [][]float32{query.Vector},
		"limit":          topK,
	}

	if query.Filter != nil {
		if filterStr, ok := query.Filter["filter"].(string); ok {
			reqBody["filter"] = filterStr
		}
	}

	respBody, err := c.doRequest(
		ctx, http.MethodPost, "/entities/search", reqBody,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	var response struct {
		Data [][]struct {
			ID       any            `json:"id"`
			Distance float32        `json:"distance"`
			Entity   map[string]any `json:"entity,omitempty"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var results []client.SearchResult
	for _, batch := range response.Data {
		for _, r := range batch {
			score := 1.0 - float64(r.Distance)
			if query.MinScore > 0 && score < query.MinScore {
				continue
			}
			results = append(results, client.SearchResult{
				ID:       fmt.Sprintf("%v", r.ID),
				Score:    float32(score),
				Metadata: r.Entity,
			})
		}
	}

	return results, nil
}

// Delete removes vectors by IDs from a collection.
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
		"dbName":         c.config.DBName,
		"collectionName": collection,
		"ids":            ids,
	}

	_, err := c.doRequest(ctx, http.MethodPost, "/entities/delete", reqBody)
	if err != nil {
		return fmt.Errorf("failed to delete entities: %w", err)
	}

	return nil
}

// Get retrieves vectors by IDs from a collection.
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
		"dbName":         c.config.DBName,
		"collectionName": collection,
		"ids":            ids,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/entities/get", reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	var response struct {
		Data []map[string]any `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	vectors := make([]client.Vector, len(response.Data))
	for i, entry := range response.Data {
		v := client.Vector{
			Metadata: make(map[string]any),
		}
		if id, ok := entry["id"]; ok {
			v.ID = fmt.Sprintf("%v", id)
		}
		if vec, ok := entry["vector"]; ok {
			if arr, ok := vec.([]any); ok {
				v.Values = make([]float32, len(arr))
				for j, val := range arr {
					if f, ok := val.(float64); ok {
						v.Values[j] = float32(f)
					}
				}
			}
		}
		for k, val := range entry {
			if k != "id" && k != "vector" {
				v.Metadata[k] = val
			}
		}
		vectors[i] = v
	}

	return vectors, nil
}

// CreateCollection creates a new collection.
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

	metricType := metricToMilvus(config.Metric)

	reqBody := map[string]any{
		"dbName":         c.config.DBName,
		"collectionName": config.Name,
		"dimension":      config.Dimension,
		"metricType":     metricType,
	}

	_, err := c.doRequest(
		ctx, http.MethodPost, "/collections/create", reqBody,
	)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

// DeleteCollection drops a collection.
func (c *Client) DeleteCollection(ctx context.Context, name string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return client.ErrNotConnected
	}

	reqBody := map[string]any{
		"dbName":         c.config.DBName,
		"collectionName": name,
	}

	_, err := c.doRequest(
		ctx, http.MethodPost, "/collections/drop", reqBody,
	)
	if err != nil {
		return fmt.Errorf("failed to drop collection: %w", err)
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

	return c.listCollections(ctx)
}

func (c *Client) listCollections(ctx context.Context) ([]string, error) {
	reqBody := map[string]any{
		"dbName": c.config.DBName,
	}

	respBody, err := c.doRequest(
		ctx, http.MethodPost, "/collections/list", reqBody,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	var response struct {
		Data []string `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Data, nil
}

func (c *Client) doRequest(
	ctx context.Context,
	method, path string,
	body any,
) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.config.GetBaseURL(), path)

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
	req.Header.Set("Accept", "application/json")

	if c.config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.Token)
	} else if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
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

	var apiResp struct {
		Code    int    `json:"code"`
		Message string `json:"message,omitempty"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err == nil && apiResp.Code != 0 {
		return nil, fmt.Errorf(
			"API error %d: %s", apiResp.Code, apiResp.Message,
		)
	}

	return respBody, nil
}

func metricToMilvus(m client.DistanceMetric) string {
	switch m {
	case client.DistanceDotProduct:
		return "IP"
	case client.DistanceEuclidean:
		return "L2"
	default:
		return "COSINE"
	}
}
