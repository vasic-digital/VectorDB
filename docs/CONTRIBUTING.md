# Contributing to VectorDB

## Overview

VectorDB (`digital.vasic.vectordb`) is a Go module providing a unified interface for vector database operations. Contributions are welcome for bug fixes, new backends, test improvements, and documentation.

## Prerequisites

- Go 1.24 or later
- `gofmt` and `goimports` installed
- `golangci-lint` installed (for linting)
- `testify` is the only test framework used

## Getting Started

1. Clone the repository using SSH:

```bash
git clone <ssh-url>
cd VectorDB
```

2. Verify the module builds and tests pass:

```bash
go build ./...
go test ./... -count=1 -race
```

## Development Workflow

### Branch Naming

Use the following prefixes:

| Prefix | Purpose |
|--------|---------|
| `feat/` | New features |
| `fix/` | Bug fixes |
| `refactor/` | Code restructuring |
| `test/` | Test additions or improvements |
| `docs/` | Documentation changes |
| `chore/` | Maintenance tasks |

Examples: `feat/weaviate-backend`, `fix/qdrant-timeout-handling`, `test/pinecone-filter-tests`.

### Commit Messages

Follow Conventional Commits format:

```
<type>(<scope>): <description>
```

**Types**: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

**Scopes**: `client`, `qdrant`, `pinecone`, `milvus`, `pgvector`, `docs`, `deps`

**Examples**:
- `feat(client): add BatchSearch interface`
- `fix(milvus): handle API error code in search response`
- `test(pgvector): add concurrent upsert test`
- `docs(qdrant): document filter syntax`

### Code Style

- Standard Go conventions per [Effective Go](https://go.dev/doc/effective_go).
- All code must be formatted with `gofmt`.
- Imports grouped with blank line separators: stdlib, third-party, internal.
- Line length should not exceed 100 characters.
- Naming conventions:
  - `camelCase` for unexported identifiers.
  - `PascalCase` for exported identifiers.
  - Acronyms in all-caps: `HTTP`, `URL`, `ID`, `API`.
  - Receiver names: 1-2 letters (e.g., `c` for client).
- Errors: always check, wrap with `fmt.Errorf("context: %w", err)`.

### Before Submitting

Run the following checks:

```bash
# Format code
gofmt -w .

# Vet for issues
go vet ./...

# Run linter
golangci-lint run

# Run all tests with race detection
go test ./... -count=1 -race
```

All checks must pass before submitting a pull request.

## Adding a New Backend

To add a new vector database backend (e.g., Weaviate, ChromaDB):

### 1. Create the Package

Create `pkg/<backend>/client.go`:

```go
package <backend>

import (
    "digital.vasic.vectordb/pkg/client"
)

// Compile-time interface checks.
var (
    _ client.VectorStore       = (*Client)(nil)
    _ client.CollectionManager = (*Client)(nil)
)
```

### 2. Implement Config

Every backend needs a `Config` struct with:

- `DefaultConfig() *Config` -- returns sensible defaults.
- `Validate() error` -- validates required fields.
- Any backend-specific helper methods (e.g., `GetBaseURL()`).

### 3. Implement Client

The `Client` struct must:

- Implement all methods of `client.VectorStore` and `client.CollectionManager`.
- Have a `connected bool` field protected by `sync.RWMutex`.
- Check `connected` in every operation and return `client.ErrNotConnected` if false.
- Use `mu.Lock()` in `Connect()` and `Close()`, `mu.RLock()` in all other methods.
- Auto-generate UUIDs for vectors with empty IDs.
- Short-circuit on empty inputs (empty vector slice, empty ID slice).

### 4. Implement Constructor

```go
func NewClient(config *Config) (*Client, error) {
    if config == nil {
        config = DefaultConfig()
    }
    if err := config.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }
    // ... create and return client
}
```

### 5. Write Tests

Create `pkg/<backend>/client_test.go` with comprehensive table-driven tests. Required test coverage:

- Config: `TestDefaultConfig`, `TestConfig_Validate` (valid + all invalid cases)
- Constructor: `TestNewClient` (nil config, valid config, invalid config)
- Connection: `TestClient_Connect_Success`, `TestClient_Connect_Error`, `TestClient_Close`
- For each of Upsert, Search, Delete, Get:
  - Success case
  - Not connected case (`ErrNotConnected`)
  - Empty input case (where applicable)
  - Server error case
  - Invalid response case
  - Auto-ID case (for Upsert)
- Collection management: CreateCollection, DeleteCollection, ListCollections
  - Success case
  - Not connected case
  - Invalid config case (for CreateCollection)
- Concurrency: at least one concurrent access test
- Metric mapping: test all distance metric conversions

Use `net/http/httptest` for REST-based backends.

### 6. Update Documentation

- Add backend section to `docs/USER_GUIDE.md`.
- Add backend section to `docs/API_REFERENCE.md`.
- Update `CLAUDE.md` package table.
- Update `AGENTS.md` with backend agent description.
- Update Mermaid diagrams in `docs/diagrams/`.

## Modifying Interfaces

Changes to `client.VectorStore` or `client.CollectionManager` are breaking changes. To modify:

1. Prefer creating a new interface (e.g., `BatchVectorStore`) over modifying existing ones.
2. If modification is necessary, update all four backend implementations.
3. Compile-time checks (`var _ Interface = (*Client)(nil)`) in each backend will catch missing implementations.
4. Update all tests to cover new/changed methods.
5. Update all documentation.

## Test Guidelines

- Use table-driven tests with descriptive names.
- Test naming: `Test<Struct>_<Method>_<Scenario>`.
- Use `testify/require` for preconditions and `testify/assert` for assertions.
- Never use `t.Fatal` or `t.FailNow` in goroutines (use channels to report failures).
- Every exported function and method must have at least one test.
- Run tests with `-race` flag to detect data races.

## Reporting Issues

When reporting bugs, include:

1. Go version (`go version`).
2. VectorDB module version.
3. Backend and version (e.g., Qdrant 1.8).
4. Minimal reproduction code.
5. Expected vs. actual behavior.
6. Full error message with stack trace if available.
