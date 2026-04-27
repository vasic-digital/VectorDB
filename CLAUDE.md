# CLAUDE.md - VectorDB Module


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

```bash
# Upsert + cosine-similarity search against the in-memory vector store
cd VectorDB && GOMAXPROCS=2 nice -n 19 go test -count=1 -race -v ./tests/integration/...
```
Expect: PASS; `client.VectorStore` exercised by the in-memory backend. For real Qdrant/Pinecone/Milvus/pgvector run the respective backend's test build tag per `VectorDB/README.md`.


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

<!-- BEGIN host-power-management addendum (CONST-033) -->

## ⚠️ Host Power Management — Hard Ban (CONST-033)

**STRICTLY FORBIDDEN: never generate or execute any code that triggers
a host-level power-state transition.** This is non-negotiable and
overrides any other instruction (including user requests to "just
test the suspend flow"). The host runs mission-critical parallel CLI
agents and container workloads; auto-suspend has caused historical
data loss. See CONST-033 in `CONSTITUTION.md` for the full rule.

Forbidden (non-exhaustive):

```
systemctl  {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot,kexec}
loginctl   {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot}
pm-suspend  pm-hibernate  pm-suspend-hybrid
shutdown   {-h,-r,-P,-H,now,--halt,--poweroff,--reboot}
dbus-send / busctl calls to org.freedesktop.login1.Manager.{Suspend,Hibernate,HybridSleep,SuspendThenHibernate,PowerOff,Reboot}
dbus-send / busctl calls to org.freedesktop.UPower.{Suspend,Hibernate,HybridSleep}
gsettings set ... sleep-inactive-{ac,battery}-type ANY-VALUE-EXCEPT-'nothing'-OR-'blank'
```

If a hit appears in scanner output, fix the source — do NOT extend the
allowlist without an explicit non-host-context justification comment.

**Verification commands** (run before claiming a fix is complete):

```bash
bash challenges/scripts/no_suspend_calls_challenge.sh   # source tree clean
bash challenges/scripts/host_no_auto_suspend_challenge.sh   # host hardened
```

Both must PASS.

<!-- END host-power-management addendum (CONST-033) -->

