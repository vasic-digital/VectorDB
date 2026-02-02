# VectorDB

Generic, reusable Go module for vector database operations. Provides a unified interface across multiple vector database backends.

## Supported Backends

- **Qdrant** - High-performance vector search engine (REST API)
- **Pinecone** - Managed vector database service (REST API)
- **Milvus** - Open-source vector database (REST API v2)
- **pgvector** - PostgreSQL extension for vector similarity search (SQL)

## Installation

```bash
go get digital.vasic.vectordb
```

## Quick Start

```go
import "digital.vasic.vectordb/pkg/client"

// All backends implement these interfaces
var store client.VectorStore
var manager client.CollectionManager
```

## Interfaces

### VectorStore

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

### CollectionManager

```go
type CollectionManager interface {
    CreateCollection(ctx context.Context, config CollectionConfig) error
    DeleteCollection(ctx context.Context, name string) error
    ListCollections(ctx context.Context) ([]string, error)
}
```

## Testing

```bash
go test ./... -count=1 -race
```

## License

Proprietary - All rights reserved.
