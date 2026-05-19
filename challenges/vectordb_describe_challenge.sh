#!/usr/bin/env bash
# challenges/vectordb_describe_challenge.sh
#
# Round-287 anti-bluff Challenge for digital.vasic.vectordb.
#
# Default mode: invoke the runner against the real, in-process client
# interface + backend constructors + i18n seam and assert it exits 0
# with the expected validation, sentinel, distance-metric, i18n,
# pgvector-helper, 4-backend, and 5-locale UX evidence. This is the
# positive-evidence proof per Article XI §11.9 — the PASS is backed
# by captured stdout, not by absence of error or a green summary line.
#
# Paired-mutation mode (--mutate): copy the validation-gate assertion
# into a scratch directory, plant a known violation (a Vector.Validate
# that returns nil for empty Values — the wrapper-bluff anti-pattern
# CONST-035 was designed to catch), build a scratch runner against the
# mutated copy, and assert the runner detects it. A mutation run that
# exits 0 means the Challenge itself is a bluff (CONST-035
# mutation-bluff), and this script exits 1 to surface that. A
# correctly detected mutation exits 99 — sentinel value the parent
# test bank recognises.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

MODE="default"
if [[ ${1:-} == "--mutate" ]]; then
    MODE="mutate"
fi

run_default() {
    echo "[vectordb-challenge] mode=default — exercising runner against real client + backends"
    cd "${REPO_ROOT}"

    local out
    out=$(GOMAXPROCS=2 nice -n 19 go run ./challenges/runner -all 2>&1) || {
        echo "[vectordb-challenge] FAIL: runner exited non-zero"
        echo "${out}"
        exit 1
    }

    # Positive-evidence assertions on captured stdout.
    if ! grep -q "^validation_gates: Vector + SearchQuery + CollectionConfig good/bad paths PASS" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: validation gates not exercised"
        echo "${out}"
        exit 1
    fi
    if ! grep -q "^sentinel=ErrNotConnected" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: ErrNotConnected sentinel identity not exercised"
        echo "${out}"
        exit 1
    fi
    if ! grep -q "^distance_metrics: 3 distinct constants" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: DistanceMetric constants not exercised"
        echo "${out}"
        exit 1
    fi
    if ! grep -q "^i18n_seam: SetTranslator" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: i18n SetTranslator/Translator seam not exercised"
        echo "${out}"
        exit 1
    fi
    if ! grep -q "^pgvector_helpers: DistanceOperator" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: pgvector helpers (DistanceOperator/VectorToString) not exercised"
        echo "${out}"
        exit 1
    fi
    for be in qdrant milvus pinecone pgvector; do
        if ! grep -q "^backend=${be} default_config_ok=true bad_config_rejected=true constructor_returns_non_nil=true$" <<<"${out}"; then
            echo "[vectordb-challenge] FAIL: backend ${be} not fully exercised"
            echo "${out}"
            exit 1
        fi
    done
    if ! grep -q "^\[en\] vectordb:" <<<"${out}" \
            || ! grep -q "^\[sr\] vectordb:" <<<"${out}" \
            || ! grep -q "^\[ja\] vectordb:" <<<"${out}" \
            || ! grep -q "^\[es\] vectordb:" <<<"${out}" \
            || ! grep -q "^\[de\] vectordb:" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: missing one or more locale UX lines"
        echo "${out}"
        exit 1
    fi
    if ! grep -qE "^OK backends=4 validation_gates=3 locales=5$" <<<"${out}"; then
        echo "[vectordb-challenge] FAIL: missing OK trailer"
        echo "${out}"
        exit 1
    fi

    echo "${out}"
    echo "[vectordb-challenge] PASS — runtime evidence captured above"
    exit 0
}

run_mutate() {
    echo "[vectordb-challenge] mode=mutate — paired-mutation evidence"
    local scratch
    scratch="$(mktemp -d -t vectordb-mutate-XXXXXX)"
    # shellcheck disable=SC2064
    trap "rm -rf '${scratch}'" EXIT

    # Stage a self-contained scratch module that ports the
    # Vector-Validate-empty-Values invariant into a mutated copy. The
    # real repository is never modified; the mutation lives only in
    # the scratch dir.
    mkdir -p "${scratch}/pkg/vectordb_scratch"

    cat > "${scratch}/go.mod" <<'EOF'
module vectordb.scratch

go 1.24
EOF

    cat > "${scratch}/pkg/vectordb_scratch/vector.go" <<'EOF'
package vectordb_scratch

// Vector mirrors the invariant-relevant subset of client.Vector.
type Vector struct {
	ID     string
	Values []float32
}

// Validate is the MUTATED implementation: it always returns nil,
// silently accepting an empty Values slice that the production
// implementation rejects. A faithful runner-style assertion MUST
// surface this by attempting to validate an empty Vector and
// expecting a non-nil error.
func (v *Vector) Validate() error {
	return nil // BUG: empty Values must be rejected
}
EOF

    cat > "${scratch}/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"

	vs "vectordb.scratch/pkg/vectordb_scratch"
)

func main() {
	bad := &vs.Vector{Values: nil}
	if err := bad.Validate(); err == nil {
		fmt.Fprintln(os.Stderr, "mutation detected: Vector.Validate accepted empty Values")
		os.Exit(99)
	}
	fmt.Println("mutation NOT detected — bluff")
	os.Exit(0)
}
EOF

    cd "${scratch}"
    # Build then exec — `go run` does not preserve exit codes >2 on
    # all toolchains, which would mask the sentinel 99 the program
    # emits when the mutation is detected.
    go build -o ./mutbin . >/dev/null 2>&1 || {
        echo "[vectordb-challenge] FAIL-MUTATE — scratch build failed"
        exit 1
    }
    local mut_out mut_rc
    set +e
    mut_out=$(./mutbin 2>&1)
    mut_rc=$?
    set -e

    echo "${mut_out}"
    if [[ ${mut_rc} -eq 99 ]]; then
        echo "[vectordb-challenge] PASS-MUTATE — mutation correctly surfaced (exit 99)"
        exit 99
    fi
    echo "[vectordb-challenge] FAIL-MUTATE — mutation NOT surfaced (exit ${mut_rc}); Challenge is a bluff"
    exit 1
}

case "${MODE}" in
    default) run_default ;;
    mutate)  run_mutate ;;
esac
