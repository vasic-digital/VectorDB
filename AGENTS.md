# AGENTS.md - VectorDB Multi-Agent Coordination Guide

## Overview

This document provides guidance for AI agents working collaboratively on the `digital.vasic.vectordb` module. It defines boundaries, responsibilities, and coordination protocols to prevent conflicts and ensure consistent development.

## Module Identity

- **Module path**: `digital.vasic.vectordb`
- **Language**: Go 1.24+
- **Dependencies**: `github.com/google/uuid`, `github.com/stretchr/testify`
- **Package count**: 5 (`client`, `qdrant`, `pinecone`, `milvus`, `pgvector`)

## Agent Roles

### Interface Agent

Responsible for the `pkg/client` package. This agent owns the core contracts that all backend adapters must satisfy.

**Scope**:
- `pkg/client/client.go` -- `VectorStore`, `CollectionManager` interfaces
- Core types: `Vector`, `SearchResult`, `SearchQuery`, `CollectionConfig`, `DistanceMetric`
- Sentinel errors: `ErrNotConnected`
- Validation methods on all types

**Rules**:
- Any change to `VectorStore` or `CollectionManager` interfaces requires coordination with all Backend Agents.
- New methods on interfaces are breaking changes. Prefer new interfaces (e.g., `BatchVectorStore`) over modifying existing ones.
- All types must have `Validate()` methods.
- JSON struct tags use `snake_case`.

### Backend Agent (Qdrant)

Responsible for the `pkg/qdrant` package.

**Scope**:
- `pkg/qdrant/config.go` -- `Config`, `DefaultConfig()`, `Validate()`, `GetHTTPURL()`, `GetGRPCAddress()`
- `pkg/qdrant/client.go` -- `Client` struct implementing `VectorStore` and `CollectionManager`
- REST API communication via `net/http`

**Key details**:
- Uses Qdrant REST API with `api-key` header for authentication.
- Distance metric mapping: Cosine -> "Cosine", DotProduct -> "Dot", Euclidean -> "Euclid".
- Compile-time interface checks are mandatory.
- All methods must check `c.connected` and return `client.ErrNotConnected` if false.
- Thread safety via `sync.RWMutex` (`mu` field).

### Backend Agent (Pinecone)

Responsible for the `pkg/pinecone` package.

**Scope**:
- `pkg/pinecone/client.go` -- `Config`, `Client` struct

**Key details**:
- Pinecone maps "collections" to namespaces. The `collection` parameter in all methods is passed as a Pinecone namespace.
- `CreateCollection` is a no-op (namespaces are created implicitly on first upsert).
- `DeleteCollection` sends a `deleteAll` request for the namespace.
- `ListCollections` queries `/describe_index_stats` and extracts namespace keys.
- Authentication via `Api-Key` header.
- `MinScore` filtering is done client-side after receiving results from the API.

### Backend Agent (Milvus)

Responsible for the `pkg/milvus` package.

**Scope**:
- `pkg/milvus/client.go` -- `Config`, `Client` struct

**Key details**:
- Uses Milvus REST API v2 at `/v2/vectordb/*` path prefix.
- Distance metric mapping: Cosine -> "COSINE", DotProduct -> "IP", Euclidean -> "L2".
- Supports Bearer token auth or HTTP Basic auth (username/password).
- Milvus metadata is stored as flat fields alongside `id` and `vector` in each entity.
- API responses include a `code` field; non-zero codes indicate errors.
- Score is computed as `1.0 - distance`.

### Backend Agent (pgvector)

Responsible for the `pkg/pgvector` package.

**Scope**:
- `pkg/pgvector/client.go` -- `Config`, `Client`, `DBPool` interface, `Row` interface, utility functions

**Key details**:
- Collections are mapped to database tables with configurable prefix (`Config.TablePrefix`).
- Schema per table: `id TEXT PK, embedding vector(N), metadata JSONB, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ`.
- `Connect()` ensures the `vector` extension is enabled via `CREATE EXTENSION IF NOT EXISTS vector`.
- Requires `SetPool(pool DBPool)` to be called before `Connect()`.
- `DBPool` and `Row` interfaces abstract the database layer for testability.
- Distance operators: Cosine -> `<=>`, DotProduct -> `<#>`, Euclidean -> `<->`.
- `Search`, `Get`, and `ListCollections` require a live database and return errors in unit test context.

### Test Agent

Responsible for ensuring test quality across all packages.

**Rules**:
- All tests are table-driven using `testify/assert` and `testify/require`.
- Test naming convention: `Test<Struct>_<Method>_<Scenario>`.
- HTTP-based backends use `httptest.Server` for unit tests.
- pgvector uses mock `DBPool` implementations.
- Every exported function must have at least one test.
- Connection state tests are mandatory: every VectorStore/CollectionManager method must test `NotConnected` behavior.
- Empty input tests are mandatory: `Upsert` with empty vectors, `Delete` with empty IDs, `Get` with empty IDs.
- Race detection: `go test -race` must pass.

## Coordination Protocols

### Interface Changes

1. Interface Agent proposes changes in a new branch.
2. All Backend Agents are notified and must update their implementations.
3. Compile-time interface checks (`var _ client.VectorStore = (*Client)(nil)`) ensure compliance.
4. All tests must pass before merging.

### New Backend Addition

1. Create `pkg/<backend>/client.go` implementing both `VectorStore` and `CollectionManager`.
2. Include compile-time interface checks.
3. Provide `Config` struct with `DefaultConfig()`, `Validate()`, and sensible defaults.
4. Provide `NewClient(config *Config) (*Client, error)` constructor.
5. All methods must guard on `connected` state.
6. Use `sync.RWMutex` for thread safety.
7. Create comprehensive tests following the existing test patterns.

### Cross-Package Dependencies

```
pkg/client  <--  pkg/qdrant
            <--  pkg/pinecone
            <--  pkg/milvus
            <--  pkg/pgvector
```

Backend packages depend only on `pkg/client`. There are no dependencies between backend packages. This must be preserved.

### Commit Conventions

- Format: `<type>(<scope>): <description>`
- Scopes: `client`, `qdrant`, `pinecone`, `milvus`, `pgvector`, `docs`, `deps`
- Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`
- Examples:
  - `feat(client): add BatchSearch interface`
  - `fix(qdrant): handle timeout on health check`
  - `test(pinecone): add min score filtering test`

## Build and Test Commands

```bash
go build ./...                          # Build all packages
go test ./... -count=1 -race            # All tests with race detection
go test ./pkg/client/... -v             # Client package tests
go test ./pkg/qdrant/... -v             # Qdrant tests
go test ./pkg/pinecone/... -v           # Pinecone tests
go test ./pkg/milvus/... -v             # Milvus tests
go test ./pkg/pgvector/... -v           # pgvector tests
go test -tags=integration ./...         # Integration tests (requires backends)
```

## File Ownership

| File | Owner |
|------|-------|
| `pkg/client/client.go` | Interface Agent |
| `pkg/client/client_test.go` | Interface Agent / Test Agent |
| `pkg/qdrant/config.go` | Backend Agent (Qdrant) |
| `pkg/qdrant/client.go` | Backend Agent (Qdrant) |
| `pkg/qdrant/client_test.go` | Backend Agent (Qdrant) / Test Agent |
| `pkg/pinecone/client.go` | Backend Agent (Pinecone) |
| `pkg/pinecone/client_test.go` | Backend Agent (Pinecone) / Test Agent |
| `pkg/milvus/client.go` | Backend Agent (Milvus) |
| `pkg/milvus/client_test.go` | Backend Agent (Milvus) / Test Agent |
| `pkg/pgvector/client.go` | Backend Agent (pgvector) |
| `pkg/pgvector/client_test.go` | Backend Agent (pgvector) / Test Agent |
| `CLAUDE.md` | Any agent (with review) |
| `AGENTS.md` | Any agent (with review) |
| `docs/*` | Any agent |

<!-- BEGIN host-power-management addendum (CONST-033) -->

## Host Power Management — Hard Ban (CONST-033)

**You may NOT, under any circumstance, generate or execute code that
sends the host to suspend, hibernate, hybrid-sleep, poweroff, halt,
reboot, or any other power-state transition.** This rule applies to:

- Every shell command you run via the Bash tool.
- Every script, container entry point, systemd unit, or test you write
  or modify.
- Every CLI suggestion, snippet, or example you emit.

**Forbidden invocations** (non-exhaustive — see CONST-033 in
`CONSTITUTION.md` for the full list):

- `systemctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot|kexec`
- `loginctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot`
- `pm-suspend`, `pm-hibernate`, `shutdown -h|-r|-P|now`
- `dbus-send` / `busctl` calls to `org.freedesktop.login1.Manager.Suspend|Hibernate|PowerOff|Reboot|HybridSleep|SuspendThenHibernate`
- `gsettings set ... sleep-inactive-{ac,battery}-type` to anything but `'nothing'` or `'blank'`

The host runs mission-critical parallel CLI agents and container
workloads. Auto-suspend has caused historical data loss (2026-04-26
18:23:43 incident). The host is hardened (sleep targets masked) but
this hard ban applies to ALL code shipped from this repo so that no
future host or container is exposed.

**Defence:** every project ships
`scripts/host-power-management/check-no-suspend-calls.sh` (static
scanner) and
`challenges/scripts/no_suspend_calls_challenge.sh` (challenge wrapper).
Both MUST be wired into the project's CI / `run_all_challenges.sh`.

**Full background:** `docs/HOST_POWER_MANAGEMENT.md` and `CONSTITUTION.md` (CONST-033).

<!-- END host-power-management addendum (CONST-033) -->

