# VectorDB User Guide

## Introduction

VectorDB (`digital.vasic.vectordb`) is a Go module that provides a unified interface for vector database operations across multiple backends. It allows you to write backend-agnostic code for vector storage, retrieval, and similarity search, then swap backends without changing your application logic.

Supported backends:

| Backend | Protocol | Default Port | Collection Concept |
|---------|----------|-------------|-------------------|
| Qdrant | REST API | 6333 (HTTP), 6334 (gRPC) | Collections |
| Pinecone | REST API | N/A (managed) | Namespaces |
| Milvus | REST API v2 | 19530 | Collections |
| pgvector | SQL | 5432 | Tables (prefixed) |

## Installation

```bash
go get digital.vasic.vectordb
```

Requires Go 1.24 or later.

## Core Concepts

### Interfaces

All backends implement two interfaces defined in `pkg/client`:

```go
import "digital.vasic.vectordb/pkg/client"

// VectorStore handles vector CRUD and search operations.
type VectorStore interface {
    Connect(ctx context.Context) error
    Close() error
    Upsert(ctx context.Context, collection string, vectors []Vector) error
    Search(ctx context.Context, collection string, query SearchQuery) ([]SearchResult, error)
    Delete(ctx context.Context, collection string, ids []string) error
    Get(ctx context.Context, collection string, ids []string) ([]Vector, error)
}

// CollectionManager handles collection lifecycle.
type CollectionManager interface {
    CreateCollection(ctx context.Context, config CollectionConfig) error
    DeleteCollection(ctx context.Context, name string) error
    ListCollections(ctx context.Context) ([]string, error)
}
```

### Types

**Vector** -- A vector with an ID, float32 values, and optional metadata:

```go
v := client.Vector{
    ID:       "doc-123",
    Values:   []float32{0.1, 0.2, 0.3, /* ... */},
    Metadata: map[string]any{"source": "web", "title": "Example"},
}
```

If `ID` is empty, all backends auto-generate a UUID.

**SearchQuery** -- Parameters for similarity search:

```go
q := client.SearchQuery{
    Vector:   []float32{0.1, 0.2, 0.3},  // query vector
    TopK:     10,                          // number of results
    Filter:   map[string]any{"key": "val"}, // optional filter
    MinScore: 0.7,                         // optional minimum score
}
```

**CollectionConfig** -- Parameters for creating a collection:

```go
cfg := client.CollectionConfig{
    Name:      "my-embeddings",
    Dimension: 1536,
    Metric:    client.DistanceCosine,
}
```

**DistanceMetric** -- Supported distance functions:

| Constant | Value | Description |
|----------|-------|-------------|
| `DistanceCosine` | `"cosine"` | Cosine similarity (default) |
| `DistanceDotProduct` | `"dot_product"` | Dot product similarity |
| `DistanceEuclidean` | `"euclidean"` | Euclidean (L2) distance |

### Validation

All core types have `Validate()` methods:

```go
v := client.Vector{ID: "v1", Values: []float32{}}
if err := v.Validate(); err != nil {
    // "vector values must not be empty"
}

q := client.SearchQuery{Vector: []float32{0.1}, TopK: 0}
if err := q.Validate(); err != nil {
    // "top_k must be positive"
}

cfg := client.CollectionConfig{Name: "", Dimension: 1536}
if err := cfg.Validate(); err != nil {
    // "collection name is required"
}
```

### Error Handling

All backends return `client.ErrNotConnected` when operations are attempted before calling `Connect()`:

```go
import "errors"

err := store.Upsert(ctx, "coll", vectors)
if errors.Is(err, client.ErrNotConnected) {
    // handle not connected
}
```

---

## Backend: Qdrant

### Configuration

```go
import "digital.vasic.vectordb/pkg/qdrant"

// Use defaults (localhost:6333, cosine, 30s timeout)
config := qdrant.DefaultConfig()

// Or customize
config := &qdrant.Config{
    Host:            "qdrant.example.com",
    HTTPPort:        6333,
    GRPCPort:        6334,
    APIKey:          "your-api-key",
    TLS:             true,
    Timeout:         60 * time.Second,
    DefaultDistance: client.DistanceCosine,
}
```

### Usage

```go
c, err := qdrant.NewClient(config)
if err != nil {
    log.Fatal(err)
}

ctx := context.Background()

// Connect (performs health check)
if err := c.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer c.Close()

// Create a collection
err = c.CreateCollection(ctx, client.CollectionConfig{
    Name:      "documents",
    Dimension: 1536,
    Metric:    client.DistanceCosine,
})

// Upsert vectors
err = c.Upsert(ctx, "documents", []client.Vector{
    {
        ID:       "doc-1",
        Values:   embedding, // []float32 of length 1536
        Metadata: map[string]any{"title": "Introduction"},
    },
})

// Search
results, err := c.Search(ctx, "documents", client.SearchQuery{
    Vector:   queryEmbedding,
    TopK:     5,
    MinScore: 0.7,
})
for _, r := range results {
    fmt.Printf("ID: %s, Score: %.4f\n", r.ID, r.Score)
}

// Get by IDs
vectors, err := c.Get(ctx, "documents", []string{"doc-1"})

// Delete
err = c.Delete(ctx, "documents", []string{"doc-1"})

// List collections
names, err := c.ListCollections(ctx)

// Delete collection
err = c.DeleteCollection(ctx, "documents")
```

### Qdrant Filter Example

Qdrant filters follow the Qdrant filter format:

```go
results, err := c.Search(ctx, "documents", client.SearchQuery{
    Vector: queryEmbedding,
    TopK:   10,
    Filter: map[string]any{
        "must": []any{
            map[string]any{
                "key":   "category",
                "match": map[string]any{"value": "science"},
            },
        },
    },
})
```

---

## Backend: Pinecone

### Configuration

```go
import "digital.vasic.vectordb/pkg/pinecone"

config := &pinecone.Config{
    APIKey:    "your-pinecone-api-key",
    IndexHost: "https://my-index-abc123.svc.us-east1-gcp.pinecone.io",
    Timeout:   30 * time.Second,
}
```

Both `APIKey` and `IndexHost` are required.

### Usage

```go
c, err := pinecone.NewClient(config)
if err != nil {
    log.Fatal(err)
}

ctx := context.Background()
if err := c.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer c.Close()

// CreateCollection is a no-op for Pinecone. Namespaces are
// created automatically on first upsert.
_ = c.CreateCollection(ctx, client.CollectionConfig{
    Name:      "my-namespace",
    Dimension: 1536,
    Metric:    client.DistanceCosine,
})

// Upsert (collection = Pinecone namespace)
err = c.Upsert(ctx, "my-namespace", []client.Vector{
    {ID: "vec-1", Values: embedding},
})

// Search with minimum score filtering
results, err := c.Search(ctx, "my-namespace", client.SearchQuery{
    Vector:   queryEmbedding,
    TopK:     10,
    MinScore: 0.8, // Client-side filtering
})

// Fetch vectors by ID
vectors, err := c.Get(ctx, "my-namespace", []string{"vec-1"})

// Delete vectors
err = c.Delete(ctx, "my-namespace", []string{"vec-1"})

// List all namespaces
namespaces, err := c.ListCollections(ctx)

// Delete namespace (deletes all vectors within it)
err = c.DeleteCollection(ctx, "my-namespace")
```

### Pinecone Notes

- The `collection` parameter in all methods maps to a Pinecone namespace.
- `CreateCollection` validates the config but does not make an API call. Namespaces appear on first upsert.
- `MinScore` filtering happens client-side after results are received from Pinecone.
- `ListCollections` calls `/describe_index_stats` and returns namespace names.

---

## Backend: Milvus

### Configuration

```go
import "digital.vasic.vectordb/pkg/milvus"

// Use defaults (localhost:19530, database "default", 30s timeout)
config := milvus.DefaultConfig()

// Or customize
config := &milvus.Config{
    Host:     "milvus.example.com",
    Port:     19530,
    Username: "admin",
    Password: "secret",
    DBName:   "my_db",
    Secure:   true,
    Token:    "bearer-token", // Preferred over username/password
    Timeout:  60 * time.Second,
}
```

Authentication priority: Bearer token > HTTP Basic (username/password).

### Usage

```go
c, err := milvus.NewClient(config)
if err != nil {
    log.Fatal(err)
}

ctx := context.Background()
if err := c.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer c.Close()

// Create collection
err = c.CreateCollection(ctx, client.CollectionConfig{
    Name:      "articles",
    Dimension: 768,
    Metric:    client.DistanceEuclidean,
})

// Upsert vectors (metadata becomes flat fields in Milvus entities)
err = c.Upsert(ctx, "articles", []client.Vector{
    {
        ID:       "art-1",
        Values:   embedding,
        Metadata: map[string]any{"category": "tech", "year": 2024},
    },
})

// Search
results, err := c.Search(ctx, "articles", client.SearchQuery{
    Vector: queryEmbedding,
    TopK:   5,
    Filter: map[string]any{"filter": "category == 'tech'"},
})

// Note: Milvus scores are computed as (1.0 - distance)

// Get, Delete, ListCollections, DeleteCollection work the same way
vectors, err := c.Get(ctx, "articles", []string{"art-1"})
err = c.Delete(ctx, "articles", []string{"art-1"})
names, err := c.ListCollections(ctx)
err = c.DeleteCollection(ctx, "articles")
```

### Milvus Notes

- Uses the REST API v2 at `/v2/vectordb/*`.
- Metadata fields are stored as flat entity fields alongside `id` and `vector`.
- Filter expressions use Milvus filter syntax passed as a string in `Filter["filter"]`.
- Distance metric mapping: Cosine -> "COSINE", DotProduct -> "IP", Euclidean -> "L2".
- API errors are detected from the `code` field in responses (non-zero = error).

---

## Backend: pgvector

### Configuration

```go
import "digital.vasic.vectordb/pkg/pgvector"

config := &pgvector.Config{
    ConnectionString: "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
    TablePrefix:      "vectordb_", // Tables: vectordb_<collection>
    Timeout:          30 * time.Second,
}
```

### Usage

pgvector requires an external database pool. The `DBPool` interface abstracts the pool:

```go
// DBPool interface (typically backed by pgxpool.Pool)
type DBPool interface {
    Ping(ctx context.Context) error
    Exec(ctx context.Context, sql string, args ...any) error
    QueryRow(ctx context.Context, sql string, args ...any) Row
    Close()
}
```

Setup with a real connection pool:

```go
c, err := pgvector.NewClient(config)
if err != nil {
    log.Fatal(err)
}

// You must provide a DBPool implementation before Connect
// Example with a wrapper around pgxpool.Pool:
c.SetPool(myPoolAdapter)

ctx := context.Background()
if err := c.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer c.Close()

// Create collection (creates a table)
err = c.CreateCollection(ctx, client.CollectionConfig{
    Name:      "embeddings",
    Dimension: 768,
    Metric:    client.DistanceCosine,
})
// Creates: vectordb_embeddings (id TEXT PK, embedding vector(768), metadata JSONB, ...)

// Upsert (uses INSERT ... ON CONFLICT DO UPDATE)
err = c.Upsert(ctx, "embeddings", []client.Vector{
    {
        ID:       "doc-1",
        Values:   embedding,
        Metadata: map[string]any{"source": "web"},
    },
})

// Delete
err = c.Delete(ctx, "embeddings", []string{"doc-1"})

// Delete collection (drops the table)
err = c.DeleteCollection(ctx, "embeddings")
```

### pgvector Utility Functions

```go
// Convert float32 slice to pgvector string format
s := pgvector.VectorToString([]float32{0.1, 0.2, 0.3})
// Result: "[0.100000,0.200000,0.300000]"

// Get the pgvector distance operator for a metric
op := pgvector.DistanceOperator(client.DistanceCosine)
// Result: "<=>"

op = pgvector.DistanceOperator(client.DistanceDotProduct)
// Result: "<#>"

op = pgvector.DistanceOperator(client.DistanceEuclidean)
// Result: "<->"
```

### pgvector Table Schema

Each collection creates a table with this schema:

```sql
CREATE TABLE IF NOT EXISTS <prefix><collection> (
    id TEXT PRIMARY KEY,
    embedding vector(<dimension>),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### pgvector Notes

- `Connect()` automatically runs `CREATE EXTENSION IF NOT EXISTS vector`.
- `SetPool()` must be called before `Connect()`.
- `Search`, `Get`, and `ListCollections` require a live database connection.
- Upsert uses `ON CONFLICT (id) DO UPDATE` for idempotent writes.
- Distance operators: Cosine `<=>`, Dot Product `<#>`, Euclidean `<->`.

---

## Writing Backend-Agnostic Code

Use the interfaces to write code that works with any backend:

```go
func IndexDocuments(
    ctx context.Context,
    store client.VectorStore,
    manager client.CollectionManager,
    collName string,
    dimension int,
    documents []Document,
) error {
    // Create collection
    err := manager.CreateCollection(ctx, client.CollectionConfig{
        Name:      collName,
        Dimension: dimension,
        Metric:    client.DistanceCosine,
    })
    if err != nil {
        return fmt.Errorf("create collection: %w", err)
    }

    // Convert documents to vectors
    vectors := make([]client.Vector, len(documents))
    for i, doc := range documents {
        vectors[i] = client.Vector{
            ID:       doc.ID,
            Values:   doc.Embedding,
            Metadata: map[string]any{"title": doc.Title},
        }
    }

    // Upsert
    return store.Upsert(ctx, collName, vectors)
}

func FindSimilar(
    ctx context.Context,
    store client.VectorStore,
    collName string,
    queryVec []float32,
    topK int,
) ([]client.SearchResult, error) {
    return store.Search(ctx, collName, client.SearchQuery{
        Vector:   queryVec,
        TopK:     topK,
        MinScore: 0.5,
    })
}
```

Then instantiate with any backend:

```go
// Use Qdrant
store, _ := qdrant.NewClient(qdrant.DefaultConfig())

// Or Pinecone
store, _ := pinecone.NewClient(&pinecone.Config{
    APIKey: "key", IndexHost: "https://...",
})

// Or Milvus
store, _ := milvus.NewClient(milvus.DefaultConfig())
```

---

## Testing

Run all tests:

```bash
go test ./... -count=1 -race
```

Run unit tests only:

```bash
go test ./... -short
```

Run tests for a specific backend:

```bash
go test -v ./pkg/qdrant/...
go test -v ./pkg/pinecone/...
go test -v ./pkg/milvus/...
go test -v ./pkg/pgvector/...
```

Run integration tests (requires running backends):

```bash
go test -tags=integration ./...
```
