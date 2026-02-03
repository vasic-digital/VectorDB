# VectorDB Architecture

## Overview

VectorDB is a Go module that provides a unified abstraction layer for vector database operations. It defines backend-agnostic interfaces in a core `client` package and implements them for four distinct vector database backends: Qdrant, Pinecone, Milvus, and pgvector.

## Design Goals

1. **Backend interchangeability** -- Application code depends on interfaces, not concrete implementations.
2. **Minimal dependencies** -- Only `github.com/google/uuid` and `github.com/stretchr/testify` (test-only). No backend-specific SDKs.
3. **Thread safety** -- All client implementations are safe for concurrent use.
4. **Explicit connection lifecycle** -- Clients must be explicitly connected and closed.
5. **Consistent error semantics** -- Common sentinel errors across all backends.

## Package Structure

```
digital.vasic.vectordb/
    pkg/
        client/         Core interfaces and types
        qdrant/         Qdrant REST API adapter
        pinecone/       Pinecone REST API adapter
        milvus/         Milvus REST API v2 adapter
        pgvector/       PostgreSQL pgvector SQL adapter
```

### Dependency Graph

```
pkg/client  <--  pkg/qdrant
            <--  pkg/pinecone
            <--  pkg/milvus
            <--  pkg/pgvector
```

All backend packages depend only on `pkg/client`. There are zero dependencies between backend packages. This is enforced by Go's import system and must be preserved.

## Design Patterns

### Adapter Pattern

Each backend package is an adapter that translates the generic `VectorStore` and `CollectionManager` interfaces into backend-specific REST API calls or SQL queries.

**How it works:**

- `pkg/client` defines the target interfaces (`VectorStore`, `CollectionManager`).
- Each backend `Client` struct implements both interfaces.
- Backend-specific details (REST paths, request formats, distance metric names) are encapsulated within the adapter.

**Examples of adaptation:**

| Operation | Qdrant | Pinecone | Milvus | pgvector |
|-----------|--------|----------|--------|----------|
| Upsert | PUT `/collections/{name}/points` | POST `/vectors/upsert` | POST `/v2/vectordb/entities/insert` | `INSERT ... ON CONFLICT DO UPDATE` |
| Search | POST `/collections/{name}/points/search` | POST `/query` | POST `/v2/vectordb/entities/search` | `SELECT ... ORDER BY embedding <=> $1` |
| Delete | POST `/collections/{name}/points/delete` | POST `/vectors/delete` | POST `/v2/vectordb/entities/delete` | `DELETE FROM ... WHERE id IN (...)` |
| Create collection | PUT `/collections/{name}` | No-op (implicit) | POST `/v2/vectordb/collections/create` | `CREATE TABLE IF NOT EXISTS ...` |
| Delete collection | DELETE `/collections/{name}` | POST `/vectors/delete` (deleteAll) | POST `/v2/vectordb/collections/drop` | `DROP TABLE IF EXISTS ...` |
| Cosine metric name | "Cosine" | N/A (index-level) | "COSINE" | `<=>` operator |
| DotProduct metric name | "Dot" | N/A | "IP" | `<#>` operator |
| Euclidean metric name | "Euclid" | N/A | "L2" | `<->` operator |

### Interface Segregation

The module separates concerns into two focused interfaces rather than one large one:

- **VectorStore** -- CRUD operations on vectors (Connect, Close, Upsert, Search, Delete, Get).
- **CollectionManager** -- Collection lifecycle operations (CreateCollection, DeleteCollection, ListCollections).

All four backends implement both interfaces in a single `Client` struct. This is verified at compile time:

```go
var (
    _ client.VectorStore       = (*Client)(nil)
    _ client.CollectionManager = (*Client)(nil)
)
```

This allows consumers to accept only the interface they need:

```go
func search(store client.VectorStore, ...) { ... }
func setup(mgr client.CollectionManager, ...) { ... }
```

### Factory Pattern (Constructor)

Each backend provides a `NewClient(config *Config) (*Client, error)` constructor that:

1. Applies defaults if config is nil (where possible).
2. Validates the configuration.
3. Returns an unconnected client.

The separation of construction from connection allows configuration validation to happen early, while connection establishment can be deferred or retried.

### Repository Pattern

The `VectorStore` interface acts as a repository for vectors. It provides:

- **Upsert** -- Create or update by ID.
- **Get** -- Retrieve by ID.
- **Delete** -- Remove by ID.
- **Search** -- Query by similarity.

This maps naturally to vector database semantics where vectors are the primary entities, identified by string IDs, and queried via similarity rather than traditional predicates.

## Thread Safety

All client implementations use `sync.RWMutex` for thread safety:

- `Connect()` and `Close()` acquire a write lock (`mu.Lock()`).
- All other methods acquire a read lock (`mu.RLock()`).
- The `connected` boolean is protected by the mutex.

This allows concurrent reads (Search, Get, Upsert, Delete) while ensuring connection state changes are exclusive.

## Connection Lifecycle

```
NewClient(config)    -- creates unconnected client
    |
    v
Connect(ctx)         -- verifies connectivity, sets connected=true
    |
    v
[operations]         -- Upsert, Search, Delete, Get, Create/Delete/ListCollections
    |
    v
Close()              -- releases resources, sets connected=false
```

Every operation checks `c.connected` and returns `client.ErrNotConnected` if false. This prevents operations on uninitialized clients.

### Backend-Specific Connection Behavior

| Backend | Connect() does | Close() does |
|---------|---------------|-------------|
| Qdrant | GET health check to root URL | Sets connected=false |
| Pinecone | POST `/describe_index_stats` | Sets connected=false |
| Milvus | POST `/v2/vectordb/collections/list` | Sets connected=false |
| pgvector | Pings DB + `CREATE EXTENSION IF NOT EXISTS vector` | Closes pool, sets connected=false |

## HTTP Client Architecture (Qdrant, Pinecone, Milvus)

The three REST-based backends share a common internal pattern:

1. A private `doRequest(ctx, method, path, body) ([]byte, error)` method.
2. JSON marshaling of request bodies.
3. Authentication via headers (varies by backend).
4. Status code checking (>= 400 = error).
5. JSON unmarshaling of responses.

| Backend | Auth Header | Auth Value |
|---------|------------|------------|
| Qdrant | `api-key` | API key string |
| Pinecone | `Api-Key` | API key string |
| Milvus | `Authorization` | `Bearer <token>` or HTTP Basic |

## SQL Architecture (pgvector)

pgvector differs from the REST backends:

- Uses a `DBPool` interface instead of `net/http`.
- Collections map to database tables with a configurable prefix.
- Vectors are stored using the PostgreSQL `vector` type.
- Metadata is stored as `JSONB`.
- Upsert uses `INSERT ... ON CONFLICT (id) DO UPDATE`.

The `DBPool` and `Row` interfaces allow the adapter to work with any PostgreSQL driver:

```go
type DBPool interface {
    Ping(ctx context.Context) error
    Exec(ctx context.Context, sql string, args ...any) error
    QueryRow(ctx context.Context, sql string, args ...any) Row
    Close()
}
```

## Validation Strategy

Validation happens at multiple levels:

1. **Config validation** -- `Config.Validate()` is called in every `NewClient()` constructor. Invalid configs are rejected before any connection attempt.
2. **Type validation** -- `Vector.Validate()`, `SearchQuery.Validate()`, `CollectionConfig.Validate()` are available for consumer use and called internally where appropriate (e.g., `CreateCollection` validates the config).
3. **Connection state** -- Every operation checks `c.connected`.
4. **Empty input short-circuiting** -- `Upsert` with empty vectors, `Delete` with empty IDs, and `Get` with empty IDs return immediately without making API calls.

## Auto-ID Generation

All backends use `github.com/google/uuid` to generate IDs when a `Vector.ID` is empty:

```go
id := v.ID
if id == "" {
    id = uuid.New().String()
}
```

This ensures every vector has a unique identifier regardless of whether the caller provides one.

## Score Semantics

Different backends return scores differently:

| Backend | Score Source | Notes |
|---------|------------|-------|
| Qdrant | `score` field in response | Higher = more similar |
| Pinecone | `score` field in response | Higher = more similar |
| Milvus | Computed as `1.0 - distance` | Higher = more similar |
| pgvector | Distance value from SQL | Lower = more similar (inverted) |

The Milvus adapter normalizes the score so that higher values consistently mean greater similarity.

## Testing Architecture

- **REST backends** (Qdrant, Pinecone, Milvus): Use `net/http/httptest.Server` to mock HTTP endpoints. No external services needed for unit tests.
- **pgvector**: Uses mock implementations of `DBPool` and `Row` interfaces. No database needed for unit tests.
- **All backends**: Table-driven tests with `testify/assert` and `testify/require`. Tests verify both success paths and error conditions.
