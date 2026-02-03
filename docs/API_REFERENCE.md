# VectorDB API Reference

## Package `client`

**Import path**: `digital.vasic.vectordb/pkg/client`

Core interfaces, types, and constants for vector database operations.

---

### Interfaces

#### `VectorStore`

```go
type VectorStore interface {
    Connect(ctx context.Context) error
    Close() error
    Upsert(ctx context.Context, collection string, vectors []Vector) error
    Search(ctx context.Context, collection string, query SearchQuery) ([]SearchResult, error)
    Delete(ctx context.Context, collection string, ids []string) error
    Get(ctx context.Context, collection string, ids []string) ([]Vector, error)
}
```

| Method | Description |
|--------|-------------|
| `Connect(ctx)` | Establishes a connection to the vector database. Must be called before any other operation. |
| `Close()` | Releases resources and closes the connection. |
| `Upsert(ctx, collection, vectors)` | Inserts or updates vectors in the specified collection. No-op if vectors is empty. |
| `Search(ctx, collection, query)` | Performs vector similarity search in the specified collection. |
| `Delete(ctx, collection, ids)` | Removes vectors by IDs from the specified collection. No-op if ids is empty. |
| `Get(ctx, collection, ids)` | Retrieves vectors by IDs from the specified collection. Returns empty slice if ids is empty. |

#### `CollectionManager`

```go
type CollectionManager interface {
    CreateCollection(ctx context.Context, config CollectionConfig) error
    DeleteCollection(ctx context.Context, name string) error
    ListCollections(ctx context.Context) ([]string, error)
}
```

| Method | Description |
|--------|-------------|
| `CreateCollection(ctx, config)` | Creates a new vector collection with the given configuration. Validates config before creation. |
| `DeleteCollection(ctx, name)` | Removes a collection by name. |
| `ListCollections(ctx)` | Returns the names of all collections. |

---

### Types

#### `Vector`

```go
type Vector struct {
    ID       string         `json:"id"`
    Values   []float32      `json:"values"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | Unique identifier. Auto-generated UUID if empty. |
| `Values` | `[]float32` | Vector values (embedding). |
| `Metadata` | `map[string]any` | Optional key-value metadata. |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(v *Vector) Validate() error` | Returns error if `Values` is nil or empty. |

#### `SearchResult`

```go
type SearchResult struct {
    ID       string         `json:"id"`
    Score    float32        `json:"score"`
    Vector   []float32      `json:"vector,omitempty"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | ID of the matched vector. |
| `Score` | `float32` | Similarity score (higher = more similar). |
| `Vector` | `[]float32` | Vector values (included only if requested by backend). |
| `Metadata` | `map[string]any` | Metadata of the matched vector. |

#### `SearchQuery`

```go
type SearchQuery struct {
    Vector   []float32      `json:"vector"`
    TopK     int            `json:"top_k"`
    Filter   map[string]any `json:"filter,omitempty"`
    MinScore float64        `json:"min_score,omitempty"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| `Vector` | `[]float32` | Query vector for similarity comparison. |
| `TopK` | `int` | Maximum number of results to return. Must be positive. |
| `Filter` | `map[string]any` | Optional backend-specific filter criteria. |
| `MinScore` | `float64` | Optional minimum similarity score threshold. |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(q *SearchQuery) Validate() error` | Returns error if `Vector` is empty or `TopK` is not positive. |

#### `CollectionConfig`

```go
type CollectionConfig struct {
    Name      string         `json:"name"`
    Dimension int            `json:"dimension"`
    Metric    DistanceMetric `json:"metric"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Collection name. Required. |
| `Dimension` | `int` | Vector dimension. Must be >= 1. |
| `Metric` | `DistanceMetric` | Distance metric. Empty string is accepted (backend uses default). |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(c *CollectionConfig) Validate() error` | Returns error if name is empty, dimension < 1, or metric is invalid. |

#### `DistanceMetric`

```go
type DistanceMetric string
```

**Constants:**

| Constant | Value | Description |
|----------|-------|-------------|
| `DistanceCosine` | `"cosine"` | Cosine similarity. |
| `DistanceDotProduct` | `"dot_product"` | Dot product similarity. |
| `DistanceEuclidean` | `"euclidean"` | Euclidean (L2) distance. |

---

### Variables

#### `ErrNotConnected`

```go
var ErrNotConnected = fmt.Errorf("not connected to vector database")
```

Returned by all operations when the client is not connected.

---

## Package `qdrant`

**Import path**: `digital.vasic.vectordb/pkg/qdrant`

Qdrant vector database adapter using the REST API.

---

### Types

#### `Config`

```go
type Config struct {
    Host            string                `json:"host" yaml:"host"`
    HTTPPort        int                   `json:"http_port" yaml:"http_port"`
    GRPCPort        int                   `json:"grpc_port" yaml:"grpc_port"`
    APIKey          string                `json:"api_key" yaml:"api_key"`
    TLS             bool                  `json:"tls" yaml:"tls"`
    Timeout         time.Duration         `json:"timeout" yaml:"timeout"`
    DefaultDistance client.DistanceMetric `json:"default_distance" yaml:"default_distance"`
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Host` | `string` | `"localhost"` | Qdrant server hostname. |
| `HTTPPort` | `int` | `6333` | HTTP API port. Must be 1-65535. |
| `GRPCPort` | `int` | `6334` | gRPC port. Must be 1-65535. |
| `APIKey` | `string` | `""` | Optional API key for authentication. |
| `TLS` | `bool` | `false` | Enable HTTPS. |
| `Timeout` | `time.Duration` | `30s` | HTTP client timeout. Must be positive. |
| `DefaultDistance` | `client.DistanceMetric` | `DistanceCosine` | Default distance metric. |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(c *Config) Validate() error` | Validates host, ports, and timeout. |
| `GetHTTPURL` | `(c *Config) GetHTTPURL() string` | Returns the full HTTP URL (e.g., `http://localhost:6333`). |
| `GetGRPCAddress` | `(c *Config) GetGRPCAddress() string` | Returns the gRPC address (e.g., `localhost:6334`). |

#### `Client`

```go
type Client struct { /* unexported fields */ }
```

Implements `client.VectorStore` and `client.CollectionManager`.

### Functions

#### `DefaultConfig`

```go
func DefaultConfig() *Config
```

Returns a Config with sensible defaults: localhost, port 6333/6334, no TLS, 30s timeout, cosine distance.

#### `NewClient`

```go
func NewClient(config *Config) (*Client, error)
```

Creates a new Qdrant client. If config is nil, uses `DefaultConfig()`. Validates config and returns error if invalid.

### Methods on Client

| Method | Signature | Description |
|--------|-----------|-------------|
| `Connect` | `(c *Client) Connect(ctx context.Context) error` | Performs health check via GET to root URL. |
| `Close` | `(c *Client) Close() error` | Sets connected to false. |
| `Upsert` | `(c *Client) Upsert(ctx, collection, vectors) error` | PUT to `/collections/{name}/points`. |
| `Search` | `(c *Client) Search(ctx, collection, query) ([]SearchResult, error)` | POST to `/collections/{name}/points/search`. |
| `Delete` | `(c *Client) Delete(ctx, collection, ids) error` | POST to `/collections/{name}/points/delete`. |
| `Get` | `(c *Client) Get(ctx, collection, ids) ([]Vector, error)` | POST to `/collections/{name}/points`. |
| `CreateCollection` | `(c *Client) CreateCollection(ctx, config) error` | PUT to `/collections/{name}`. |
| `DeleteCollection` | `(c *Client) DeleteCollection(ctx, name) error` | DELETE to `/collections/{name}`. |
| `ListCollections` | `(c *Client) ListCollections(ctx) ([]string, error)` | GET to `/collections`. |

---

## Package `pinecone`

**Import path**: `digital.vasic.vectordb/pkg/pinecone`

Pinecone managed vector database adapter using the REST API.

---

### Types

#### `Config`

```go
type Config struct {
    APIKey      string        `json:"api_key"`
    Environment string        `json:"environment"`
    IndexHost   string        `json:"index_host"`
    Timeout     time.Duration `json:"timeout"`
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `APIKey` | `string` | `""` | Pinecone API key. Required. |
| `Environment` | `string` | `""` | Pinecone environment identifier. |
| `IndexHost` | `string` | `""` | Full index host URL. Required. |
| `Timeout` | `time.Duration` | `30s` | HTTP client timeout. |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(c *Config) Validate() error` | Validates APIKey and IndexHost are non-empty. |

#### `Client`

```go
type Client struct { /* unexported fields */ }
```

Implements `client.VectorStore` and `client.CollectionManager`.

### Functions

#### `DefaultConfig`

```go
func DefaultConfig() *Config
```

Returns a Config with 30s timeout. APIKey and IndexHost must be set manually.

#### `NewClient`

```go
func NewClient(config *Config) (*Client, error)
```

Creates a new Pinecone client. If config is nil, uses `DefaultConfig()` (will fail validation since APIKey is empty). Validates config and returns error if invalid.

### Methods on Client

| Method | Signature | Description |
|--------|-----------|-------------|
| `Connect` | `(c *Client) Connect(ctx) error` | POST to `/describe_index_stats`. |
| `Close` | `(c *Client) Close() error` | Sets connected to false. |
| `Upsert` | `(c *Client) Upsert(ctx, collection, vectors) error` | POST to `/vectors/upsert`. Collection = namespace. |
| `Search` | `(c *Client) Search(ctx, collection, query) ([]SearchResult, error)` | POST to `/query`. MinScore filtered client-side. |
| `Delete` | `(c *Client) Delete(ctx, collection, ids) error` | POST to `/vectors/delete`. |
| `Get` | `(c *Client) Get(ctx, collection, ids) ([]Vector, error)` | GET to `/vectors/fetch?ids=...&namespace=...`. |
| `CreateCollection` | `(c *Client) CreateCollection(ctx, config) error` | No-op (validates config only). Namespaces are implicit. |
| `DeleteCollection` | `(c *Client) DeleteCollection(ctx, name) error` | POST to `/vectors/delete` with `deleteAll: true`. |
| `ListCollections` | `(c *Client) ListCollections(ctx) ([]string, error)` | POST to `/describe_index_stats`, extracts namespace keys. |

---

## Package `milvus`

**Import path**: `digital.vasic.vectordb/pkg/milvus`

Milvus vector database adapter using the REST API v2.

---

### Types

#### `Config`

```go
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
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Host` | `string` | `"localhost"` | Milvus server hostname. Required. |
| `Port` | `int` | `19530` | Server port. Must be positive. |
| `Username` | `string` | `""` | Username for Basic auth. |
| `Password` | `string` | `""` | Password for Basic auth. |
| `DBName` | `string` | `"default"` | Database name. |
| `Secure` | `bool` | `false` | Enable HTTPS. |
| `Token` | `string` | `""` | Bearer token (preferred over Basic auth). |
| `Timeout` | `time.Duration` | `30s` | HTTP client timeout. |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(c *Config) Validate() error` | Validates host is non-empty and port is positive. |
| `GetBaseURL` | `(c *Config) GetBaseURL() string` | Returns the base URL (e.g., `http://localhost:19530/v2/vectordb`). |

#### `Client`

```go
type Client struct { /* unexported fields */ }
```

Implements `client.VectorStore` and `client.CollectionManager`.

### Functions

#### `DefaultConfig`

```go
func DefaultConfig() *Config
```

Returns a Config with defaults: localhost, port 19530, database "default", 30s timeout.

#### `NewClient`

```go
func NewClient(config *Config) (*Client, error)
```

Creates a new Milvus client. If config is nil, uses `DefaultConfig()`. Validates config and returns error if invalid.

### Methods on Client

| Method | Signature | Description |
|--------|-----------|-------------|
| `Connect` | `(c *Client) Connect(ctx) error` | Lists collections to verify connectivity. |
| `Close` | `(c *Client) Close() error` | Sets connected to false. |
| `Upsert` | `(c *Client) Upsert(ctx, collection, vectors) error` | POST to `/entities/insert`. Metadata stored as flat fields. |
| `Search` | `(c *Client) Search(ctx, collection, query) ([]SearchResult, error)` | POST to `/entities/search`. Score = 1.0 - distance. |
| `Delete` | `(c *Client) Delete(ctx, collection, ids) error` | POST to `/entities/delete`. |
| `Get` | `(c *Client) Get(ctx, collection, ids) ([]Vector, error)` | POST to `/entities/get`. |
| `CreateCollection` | `(c *Client) CreateCollection(ctx, config) error` | POST to `/collections/create`. |
| `DeleteCollection` | `(c *Client) DeleteCollection(ctx, name) error` | POST to `/collections/drop`. |
| `ListCollections` | `(c *Client) ListCollections(ctx) ([]string, error)` | POST to `/collections/list`. |

---

## Package `pgvector`

**Import path**: `digital.vasic.vectordb/pkg/pgvector`

PostgreSQL pgvector extension adapter using SQL queries.

---

### Types

#### `Config`

```go
type Config struct {
    ConnectionString string        `json:"connection_string"`
    TablePrefix      string        `json:"table_prefix"`
    Timeout          time.Duration `json:"timeout"`
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ConnectionString` | `string` | `""` | PostgreSQL connection string. Required. |
| `TablePrefix` | `string` | `"vectordb_"` | Prefix for collection table names. |
| `Timeout` | `time.Duration` | `30s` | Operation timeout. |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(c *Config) Validate() error` | Validates ConnectionString is non-empty. |

#### `DBPool`

```go
type DBPool interface {
    Ping(ctx context.Context) error
    Exec(ctx context.Context, sql string, args ...any) error
    QueryRow(ctx context.Context, sql string, args ...any) Row
    Close()
}
```

Abstracts the database connection pool. Typically backed by `pgxpool.Pool` with a thin wrapper.

#### `Row`

```go
type Row interface {
    Scan(dest ...any) error
}
```

Abstracts a single row result from a query.

#### `Client`

```go
type Client struct { /* unexported fields */ }
```

Implements `client.VectorStore` and `client.CollectionManager`.

### Functions

#### `DefaultConfig`

```go
func DefaultConfig() *Config
```

Returns a Config with defaults: prefix `"vectordb_"`, 30s timeout. ConnectionString must be set manually.

#### `NewClient`

```go
func NewClient(config *Config) (*Client, error)
```

Creates a new pgvector client. If config is nil, uses `DefaultConfig()` (will fail validation since ConnectionString is empty). Validates config and returns error if invalid.

#### `DistanceOperator`

```go
func DistanceOperator(m client.DistanceMetric) string
```

Returns the pgvector SQL operator for the given distance metric.

| Input | Output |
|-------|--------|
| `DistanceCosine` | `<=>` |
| `DistanceDotProduct` | `<#>` |
| `DistanceEuclidean` | `<->` |
| (empty/default) | `<=>` |

#### `VectorToString`

```go
func VectorToString(v []float32) string
```

Converts a float32 slice to pgvector string format: `"[0.100000,0.200000,0.300000]"`.

### Methods on Client

| Method | Signature | Description |
|--------|-----------|-------------|
| `SetPool` | `(c *Client) SetPool(pool DBPool)` | Sets the database pool. Must be called before `Connect`. |
| `Connect` | `(c *Client) Connect(ctx) error` | Pings DB, creates vector extension. Requires pool to be set. |
| `Close` | `(c *Client) Close() error` | Closes pool and sets connected to false. |
| `Upsert` | `(c *Client) Upsert(ctx, collection, vectors) error` | `INSERT ... ON CONFLICT DO UPDATE`. |
| `Search` | `(c *Client) Search(ctx, collection, query) ([]SearchResult, error)` | Requires live DB. |
| `Delete` | `(c *Client) Delete(ctx, collection, ids) error` | `DELETE FROM ... WHERE id IN (...)`. |
| `Get` | `(c *Client) Get(ctx, collection, ids) ([]Vector, error)` | Requires live DB. |
| `CreateCollection` | `(c *Client) CreateCollection(ctx, config) error` | `CREATE TABLE IF NOT EXISTS ...` with vector column. |
| `DeleteCollection` | `(c *Client) DeleteCollection(ctx, name) error` | `DROP TABLE IF EXISTS ...`. |
| `ListCollections` | `(c *Client) ListCollections(ctx) ([]string, error)` | Requires live DB. |
