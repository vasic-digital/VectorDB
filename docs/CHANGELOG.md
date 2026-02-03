# Changelog

All notable changes to the VectorDB module will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [v0.1.0] - 2025-01-01

### Added

- **Core interfaces** (`pkg/client`):
  - `VectorStore` interface with Connect, Close, Upsert, Search, Delete, Get methods.
  - `CollectionManager` interface with CreateCollection, DeleteCollection, ListCollections methods.
  - `Vector` type with ID, Values, and Metadata fields.
  - `SearchResult` type with ID, Score, Vector, and Metadata fields.
  - `SearchQuery` type with Vector, TopK, Filter, and MinScore fields.
  - `CollectionConfig` type with Name, Dimension, and Metric fields.
  - `DistanceMetric` type with Cosine, DotProduct, and Euclidean constants.
  - `ErrNotConnected` sentinel error.
  - Validation methods on Vector, SearchQuery, and CollectionConfig.

- **Qdrant adapter** (`pkg/qdrant`):
  - REST API client implementing VectorStore and CollectionManager.
  - Config with Host, HTTPPort, GRPCPort, APIKey, TLS, Timeout, and DefaultDistance.
  - DefaultConfig with sensible localhost defaults.
  - Health check on Connect.
  - API key authentication via `api-key` header.
  - Distance metric mapping (Cosine, Dot, Euclid).
  - Full test suite with httptest mock server.

- **Pinecone adapter** (`pkg/pinecone`):
  - REST API client implementing VectorStore and CollectionManager.
  - Config with APIKey, Environment, IndexHost, and Timeout.
  - Collection operations mapped to Pinecone namespaces.
  - CreateCollection as no-op (namespaces are implicit).
  - Client-side MinScore filtering.
  - API key authentication via `Api-Key` header.
  - Full test suite with httptest mock server.

- **Milvus adapter** (`pkg/milvus`):
  - REST API v2 client implementing VectorStore and CollectionManager.
  - Config with Host, Port, Username, Password, DBName, Secure, Token, and Timeout.
  - DefaultConfig with sensible localhost defaults.
  - Bearer token and HTTP Basic authentication support.
  - Distance metric mapping (COSINE, IP, L2).
  - Score normalization (1.0 - distance).
  - API error code detection.
  - Full test suite with httptest mock server.

- **pgvector adapter** (`pkg/pgvector`):
  - SQL-based client implementing VectorStore and CollectionManager.
  - Config with ConnectionString, TablePrefix, and Timeout.
  - DBPool and Row interfaces for database abstraction.
  - Automatic `CREATE EXTENSION IF NOT EXISTS vector` on Connect.
  - Collection-to-table mapping with configurable prefix.
  - Table schema: id TEXT PK, embedding vector(N), metadata JSONB, timestamps.
  - Upsert with INSERT ... ON CONFLICT DO UPDATE.
  - VectorToString and DistanceOperator utility functions.
  - Full test suite with mock DBPool.

- **Common features across all backends**:
  - Thread safety via sync.RWMutex.
  - Compile-time interface compliance checks.
  - Auto-generated UUIDs for vectors with empty IDs.
  - Empty input short-circuiting.
  - Connection state validation on all operations.
  - Comprehensive unit tests with race detection.
