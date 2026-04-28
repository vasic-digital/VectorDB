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



<!-- CONST-035 anti-bluff addendum (cascaded) -->

## CONST-035 — Anti-Bluff Tests & Challenges (mandatory; inherits from root)

Tests and Challenges in this submodule MUST verify the product, not
the LLM's mental model of the product. A test that passes when the
feature is broken is worse than a missing test — it gives false
confidence and lets defects ship to users. Functional probes at the
protocol layer are mandatory:

- TCP-open is the FLOOR, not the ceiling. Postgres → execute
  `SELECT 1`. Redis → `PING` returns `PONG`. ChromaDB → `GET
  /api/v1/heartbeat` returns 200. MCP server → TCP connect + valid
  JSON-RPC handshake. HTTP gateway → real request, real response,
  non-empty body.
- Container `Up` is NOT application healthy. A `docker/podman ps`
  `Up` status only means PID 1 is running; the application may be
  crash-looping internally.
- No mocks/fakes outside unit tests (already CONST-030; CONST-035
  raises the cost of a mock-driven false pass to the same severity
  as a regression).
- Re-verify after every change. Don't assume a previously-passing
  test still verifies the same scope after a refactor.
- Verification of CONST-035 itself: deliberately break the feature
  (e.g. `kill <service>`, swap a password). The test MUST fail. If
  it still passes, the test is non-conformant and MUST be tightened.

## CONST-033 clarification — distinguishing host events from sluggishness

Heavy container builds (BuildKit pulling many GB of layers, parallel
podman/docker compose-up across many services) can make the host
**appear** unresponsive — high load average, slow SSH, watchers
timing out. **This is NOT a CONST-033 violation.** Suspend / hibernate
/ logout are categorically different events. Distinguish via:

- `uptime` — recent boot? if so, the host actually rebooted.
- `loginctl list-sessions` — session(s) still active? if yes, no logout.
- `journalctl ... | grep -i 'will suspend\|hibernate'` — zero broadcasts
  since the CONST-033 fix means no suspend ever happened.
- `dmesg | grep -i 'killed process\|out of memory'` — OOM kills are
  also NOT host-power events; they're memory-pressure-induced and
  require their own separate fix (lower per-container memory limits,
  reduce parallelism).

A sluggish host under build pressure recovers when the build finishes;
a suspended host requires explicit unsuspend (and CONST-033 should
make that impossible by hardening `IdleAction=ignore` +
`HandleSuspendKey=ignore` + masked `sleep.target`,
`suspend.target`, `hibernate.target`, `hybrid-sleep.target`).

If you observe what looks like a suspend during heavy builds, the
correct first action is **not** "edit CONST-033" but `bash
challenges/scripts/host_no_auto_suspend_challenge.sh` to confirm the
hardening is intact. If hardening is intact AND no suspend
broadcast appears in journal, the perceived event was build-pressure
sluggishness, not a power transition.

<!-- BEGIN no-session-termination addendum (CONST-036) -->

## User-Session Termination — Hard Ban (CONST-036)

**You may NOT, under any circumstance, generate or execute code that
ends the currently-logged-in user's desktop session, kills their
`user@<UID>.service` user manager, or indirectly forces them to
manually log out / power off.** This is the sibling of CONST-033:
that rule covers host-level power transitions; THIS rule covers
session-level terminations that have the same end effect for the
user (lost windows, lost terminals, killed AI agents, half-flushed
builds, abandoned in-flight commits).

**Why this rule exists.** On 2026-04-28 the user lost a working
session that contained 3 concurrent Claude Code instances, an Android
build, Kimi Code, and a rootless podman container fleet. The
`user.slice` consumed 60.6 GiB peak / 5.2 GiB swap, the GUI became
unresponsive, the user was forced to log out and then power off via
the GNOME shell. The host could not auto-suspend (CONST-033 was in
place and verified) and the kernel OOM killer never fired — but the
user had to manually end the session anyway, because nothing
prevented overlapping heavy workloads from saturating the slice.
CONST-036 closes that loophole at both the source-code layer and the
operational layer. See
`docs/issues/fixed/SESSION_LOSS_2026-04-28.md` in the HelixAgent
project.

**Forbidden direct invocations** (non-exhaustive):

- `loginctl terminate-user|terminate-session|kill-user|kill-session`
- `systemctl stop user@<UID>` / `systemctl kill user@<UID>`
- `gnome-session-quit`
- `pkill -KILL -u $USER` / `killall -u $USER`
- `dbus-send` / `busctl` calls to `org.gnome.SessionManager.Logout|Shutdown|Reboot`
- `echo X > /sys/power/state`
- `/usr/bin/poweroff`, `/usr/bin/reboot`, `/usr/bin/halt`

**Indirect-pressure clauses:**

1. Do not spawn parallel heavy workloads casually; check `free -h`
   first; keep `user.slice` under 70% of physical RAM.
2. Long-lived background subagents go in `system.slice`. Rootless
   podman containers die with the user manager.
3. Document AI-agent concurrency caps in CLAUDE.md.
4. Never script "log out and back in" recovery flows.

**Defence:** every project ships
`scripts/host-power-management/check-no-session-termination-calls.sh`
(static scanner) and
`challenges/scripts/no_session_termination_calls_challenge.sh`
(challenge wrapper). Both MUST be wired into the project's CI /
`run_all_challenges.sh`.

<!-- END no-session-termination addendum (CONST-036) -->
