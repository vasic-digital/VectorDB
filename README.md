# VectorDB

`digital.vasic.vectordb` is a generic, reusable Go module providing a
unified interface across multiple vector database backends. The
public client surface is small, locale-aware, and fully decoupled
from any consuming project per CONST-051(B) — VectorDB never reaches
into a parent project for its catalogue, configuration, or
hostnames.

**Module**: `digital.vasic.vectordb`
**Go**: `1.25+`
**Status**: production — covered by unit tests + integration tests
(SKIP-OK behind real-Postgres env vars) + the round-287 anti-bluff
Challenge runner with paired-mutation evidence.

## Supported Backends

| Backend     | Package                  | Transport       | Status                 |
|-------------|--------------------------|-----------------|------------------------|
| **Qdrant**  | `pkg/qdrant`             | REST            | constructor + config   |
| **Pinecone**| `pkg/pinecone`           | REST            | constructor + config   |
| **Milvus**  | `pkg/milvus`             | REST v2         | constructor + config   |
| **pgvector**| `pkg/pgvector`           | SQL (pgx)       | wired Search + Get     |

All four backends implement the same `client.VectorStore` and
`client.CollectionManager` interfaces, so swapping a backend is a
config change rather than a code change for the consumer.

## Installation

```bash
go get digital.vasic.vectordb
```

## Quick Start

```go
import (
    "context"
    "digital.vasic.vectordb/pkg/client"
    "digital.vasic.vectordb/pkg/qdrant"
)

cfg := qdrant.DefaultConfig()
cfg.Host = "qdrant.internal"
cfg.APIKey = os.Getenv("QDRANT_API_KEY") // CONST-042: never hardcode secrets

store, err := qdrant.NewClient(cfg)
if err != nil { /* handle */ }
if err := store.Connect(context.Background()); err != nil { /* handle */ }
defer store.Close()

// Upsert real vectors.
err = store.Upsert(ctx, "docs", []client.Vector{
    {ID: "doc-1", Values: []float32{0.1, 0.2, 0.3}, Metadata: map[string]any{"src": "README"}},
})
```

## Public Interfaces

### `VectorStore`

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

### `CollectionManager`

```go
type CollectionManager interface {
    CreateCollection(ctx context.Context, config CollectionConfig) error
    DeleteCollection(ctx context.Context, name string) error
    ListCollections(ctx context.Context) ([]string, error)
}
```

### Validation Surface

Every input type (`Vector`, `SearchQuery`, `CollectionConfig`)
implements `Validate() error` that routes its message through the
package-scoped `i18n.Translator`. Consumers wire a locale-aware
translator via `client.SetTranslator(custom)`; the default
`i18n.NoopTranslator` returns the key verbatim per CONST-046.

```go
import "digital.vasic.vectordb/pkg/client"

client.SetTranslator(myProjectTranslator)
defer client.SetTranslator(nil) // restore noop

v := &client.Vector{Values: nil}
if err := v.Validate(); err != nil {
    // err.Error() routed through myProjectTranslator
}
```

## Anti-Bluff Guarantees (Article XI §11.9 / CONST-035)

Every PASS in this module is backed by **positive runtime evidence**,
not by absence-of-error or a green summary line. Specifically:

1. **Validation gates are symmetric.** Every `Validate()` test
   exercises BOTH a known-good input (must pass) AND a known-bad
   input (must fail). The Challenge runner re-checks both directions
   at release-gate time — mutating either direction surfaces via the
   paired-mutation exit-99 sentinel.
2. **No mocks beyond unit tests (CONST-050(A)).** Backend
   `*_integration_test.go` files probe real services (real Postgres
   for `pkg/pgvector`); they SKIP-OK with a loud marker when the
   target service is absent rather than silently passing.
3. **Constructors return non-nil clients.** Every backend
   `NewClient(DefaultConfig)` call is asserted to return a usable
   client pointer; a constructor that returns `(nil, nil)` is a
   CONST-035 silent-success violation.
4. **Bad configs are rejected.** Every backend's `Config.Validate()`
   is asserted to REJECT a known-bad input (empty host / missing API
   key / missing connection string). Acceptance of a known-bad input
   is a constructor-bluff (CONST-035) of equal severity to the
   converse.
5. **Sentinel errors have stable identity.** `client.ErrNotConnected`
   is asserted to satisfy `errors.Is(ErrNotConnected, ErrNotConnected)`
   AND carry a non-empty message — mutation of either property
   silently breaks downstream `errors.Is` switches in consumer code.
6. **i18n seam is wired end-to-end (CONST-046).** The runner installs
   a recording `Translator`, triggers a validation error, asserts the
   resulting message starts with the recording-translator's sentinel
   prefix, then restores `nil` and asserts `NoopTranslator` is
   re-installed. Mutating `SetTranslator(nil)` to no-op silently
   breaks the round-trip and surfaces here.
7. **5-locale UX evidence (CONST-046).** Every run prints
   `[en] [sr] [ja] [es] [de]` summary lines containing the canonical
   `vectordb:` token. Missing locales surface as exit-code 4.
8. **Paired-mutation evidence (CONST-035).** The describe-Challenge
   ships with `--mutate` mode that plants a `Vector.Validate` always
   returning `nil`, builds a scratch runner against the mutated copy,
   and asserts exit-99. A mutation run that exits 0 means the
   Challenge itself is a bluff and surfaces as FAIL-MUTATE.

See `docs/test-coverage.md` for the full symbol → test → Challenge
ledger covering every exported symbol of the module.

## Testing

### Unit tests (mocks permitted per CONST-050(A))

```bash
GOMAXPROCS=2 nice -n 19 go test -race -count=1 ./pkg/...
```

Expected: every package PASS in under 5 s on a developer laptop.

### Integration tests (real Postgres + pgvector)

```bash
export VECTORDB_PGVECTOR_DSN="postgres://user:pass@host:5432/db?sslmode=disable"
go test -tags=integration -race -count=1 ./pkg/pgvector/...
```

Tests SKIP-OK with a loud marker when `VECTORDB_PGVECTOR_DSN` is
unset; absence of coverage is loud rather than silent (CONST-035).

### Challenge runner (release-gate evidence)

```bash
# Default: positive-evidence run, exit 0.
bash challenges/vectordb_describe_challenge.sh

# Paired-mutation evidence, exit 99 on detected mutation.
bash challenges/vectordb_describe_challenge.sh --mutate
```

Default mode prints:
- `validation_gates: Vector + SearchQuery + CollectionConfig good/bad paths PASS (gates=3)`
- `sentinel=ErrNotConnected msg="not connected to vector database" identity_ok=true`
- `distance_metrics: 3 distinct constants (cosine=... dot=... euclidean=...)`
- `i18n_seam: SetTranslator(custom) → validation message routed; SetTranslator(nil) → noop restored`
- `pgvector_helpers: DistanceOperator(3 metrics) + VectorToString shape PASS (sample=...)`
- `backend=qdrant default_config_ok=true bad_config_rejected=true constructor_returns_non_nil=true`
- (… same line for `milvus`, `pinecone`, `pgvector`)
- `[en] vectordb: ...` through `[de] vectordb: ...`
- `OK backends=4 validation_gates=3 locales=5`

## Directory Layout

```
.
├── README.md                          # this file
├── CONSTITUTION.md / CLAUDE.md / AGENTS.md   # governance (see CONST-059)
├── helix-deps.yaml                    # CONST-054 dependency manifest
├── go.mod / go.sum
├── Makefile                           # build / test / lint targets
├── pkg/
│   ├── client/                        # public VectorStore / CollectionManager interfaces
│   ├── i18n/                          # locale-aware message seam (NoopTranslator default)
│   │   └── bundles/active.en.yaml     # English message catalogue (CONST-046 source of truth)
│   ├── qdrant/                        # Qdrant REST adapter
│   ├── pinecone/                      # Pinecone REST adapter
│   ├── milvus/                        # Milvus REST v2 adapter
│   └── pgvector/                      # PostgreSQL + pgvector SQL adapter (real pool wired)
├── tests/
│   ├── benchmark/                     # latency / throughput benchmarks
│   ├── e2e/                           # end-to-end fixtures
│   ├── integration/                   # cross-backend integration tests (SKIP-OK behind env)
│   ├── security/                      # CONST-042 secret-leak + input-fuzz tests
│   └── stress/                        # sustained-load tests
├── challenges/
│   ├── vectordb_describe_challenge.sh # round-287 describe-Challenge + --mutate
│   ├── runner/main.go                 # round-287 in-process runner
│   ├── fixtures/locales.yaml          # round-287 5-locale UX fixture
│   └── scripts/                       # legacy compile / functionality / unit Challenges
├── docs/
│   ├── API_REFERENCE.md
│   ├── ARCHITECTURE.md
│   ├── USER_GUIDE.md
│   ├── CHANGELOG.md
│   ├── CONTRIBUTING.md
│   ├── HOST_POWER_MANAGEMENT.md       # CONST-033 reference
│   ├── test-coverage.md               # round-287 symbol → test ledger
│   └── diagrams/
├── scripts/                           # helper scripts (regenerate coverage, etc.)
├── Upstreams/                         # multi-remote push recipes (install_upstreams)
└── install_upstreams.sh               # local copy for offline bootstrap
```

## Decoupling (CONST-051(B))

VectorDB has **no project-specific knowledge**. There are no
hardcoded HelixCode paths, no hardcoded hostnames, no references to
parent-project assets. All configuration enters through:
- explicit constructor parameters (`*Config` structs)
- `client.SetTranslator()` for locale overrides
- environment variables read by the consumer, then passed in

A consumer that imports `digital.vasic.vectordb` carries no
HelixCode coupling. This decoupling is enforced by the
no-fakes-beyond-unit-tests audit (CONST-050(A)) and the canonical-root
inheritance audit (CONST-059).

## Dependency Manifest (CONST-054)

VectorDB declares its own-org dependencies in `helix-deps.yaml` at
the module root. Consuming projects use
`incorporate-submodule git@github.com:vasic-digital/VectorDB.git`
which reads the manifest, fans the dependency graph out to the
parent project's canonical paths per CONST-051(C), and emits a
`.helix-manifest.yaml` audit record. No nested own-org chains.

## Host Power Management (CONST-033)

This module never invokes host power-state transitions. See
`docs/HOST_POWER_MANAGEMENT.md` and the `challenges/scripts/`
`no_suspend_calls_challenge.sh` + `host_no_auto_suspend_challenge.sh`
scripts. Both must PASS as part of any release gate.

## License

Proprietary — All rights reserved (HelixDevelopment / vasic-digital).
