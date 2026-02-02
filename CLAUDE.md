# CLAUDE.md - VectorDB Module

## Overview

`digital.vasic.vectordb` is a generic, reusable Go module for vector database operations. It provides a unified interface for multiple vector database backends including Qdrant, Pinecone, Milvus, and pgvector (PostgreSQL).

**Module**: `digital.vasic.vectordb` (Go 1.24+)

## Build & Test

```bash
go build ./...
go test ./... -count=1 -race
go test ./... -short              # Unit tests only
go test -tags=integration ./...   # Integration tests (requires backends)
```

## Code Style

- Standard Go conventions, `gofmt` formatting
- Imports grouped: stdlib, third-party, internal (blank line separated)
- Line length <= 100 chars
- Naming: `camelCase` private, `PascalCase` exported, acronyms all-caps
- Errors: always check, wrap with `fmt.Errorf("...: %w", err)`
- Tests: table-driven, `testify`, naming `Test<Struct>_<Method>_<Scenario>`

## Package Structure

| Package | Purpose |
|---------|---------|
| `pkg/client` | Core interfaces (VectorStore, CollectionManager) and types |
| `pkg/qdrant` | Qdrant adapter (REST API) |
| `pkg/pinecone` | Pinecone adapter (REST API) |
| `pkg/milvus` | Milvus adapter (REST API v2) |
| `pkg/pgvector` | pgvector PostgreSQL adapter (SQL) |

## Key Interfaces

- `client.VectorStore` -- Connect, Close, Upsert, Search, Delete, Get
- `client.CollectionManager` -- CreateCollection, DeleteCollection, ListCollections
- `client.DistanceMetric` -- Cosine, DotProduct, Euclidean

## Key Types

- `client.Vector` -- ID, Values, Metadata
- `client.SearchResult` -- ID, Score, Vector, Metadata
- `client.SearchQuery` -- Vector, TopK, Filter, MinScore
- `client.CollectionConfig` -- Name, Dimension, Metric

## Design Patterns

- **Strategy**: VectorStore implementations per backend
- **Builder**: Config chaining with `With*` methods
- **Interface Segregation**: VectorStore + CollectionManager

## Commit Style

Conventional Commits: `feat(qdrant): add batch search support`
