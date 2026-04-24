# CLAUDE.md - VectorDB Module


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

<!-- TODO: replace this block with the exact command(s) that exercise this
     module end-to-end against real dependencies, and the expected output.
     The commands must run the real artifact (built binary, deployed
     container, real service) — no in-process fakes, no mocks, no
     `httptest.NewServer`, no Robolectric, no JSDOM as proof of done. -->

```bash
# TODO
```

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

## Integration Seams

| Direction | Sibling modules |
|-----------|-----------------|
| Upstream (this module imports) | none |
| Downstream (these import this module) | HelixLLM |

*Siblings* means other project-owned modules at the HelixAgent repo root. The root HelixAgent app and external systems are not listed here — the list above is intentionally scoped to module-to-module seams, because drift *between* sibling modules is where the "tests pass, product broken" class of bug most often lives. See root `CLAUDE.md` for the rules that keep these seams contract-tested.
