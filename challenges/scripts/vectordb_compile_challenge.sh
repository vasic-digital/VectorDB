#!/usr/bin/env bash
# vectordb_compile_challenge.sh - Validates VectorDB module compilation and code quality
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="VectorDB"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Compile Challenge ==="
echo ""

# Test 1: go.mod exists
echo "Test: go.mod exists"
if [ -f "${MODULE_DIR}/go.mod" ]; then
    pass "go.mod exists"
else
    fail "go.mod missing"
fi

# Test 2: Module name is correct
echo "Test: Module name is digital.vasic.vectordb"
if grep -q "^module digital.vasic.vectordb$" "${MODULE_DIR}/go.mod"; then
    pass "Module name is digital.vasic.vectordb"
else
    fail "Module name mismatch"
fi

# Test 3: Go version is 1.24+
echo "Test: Go version is 1.24+"
if grep -qE "^go 1\.2[4-9]" "${MODULE_DIR}/go.mod"; then
    pass "Go version is 1.24+"
else
    fail "Go version is not 1.24+"
fi

# Test 4: Module compiles
echo "Test: Module compiles"
if (cd "${MODULE_DIR}" && go build ./... 2>/dev/null); then
    pass "Module compiles successfully"
else
    fail "Module compilation failed"
fi

# Test 5: go vet passes
echo "Test: go vet passes"
if (cd "${MODULE_DIR}" && go vet ./... 2>/dev/null); then
    pass "go vet passes"
else
    fail "go vet found issues"
fi

# Test 6: Documentation exists
echo "Test: Required documentation exists"
docs_ok=true
for doc in README.md CLAUDE.md AGENTS.md; do
    if [ ! -f "${MODULE_DIR}/${doc}" ]; then
        fail "Missing ${doc}"
        docs_ok=false
    fi
done
if [ "$docs_ok" = true ]; then
    pass "All documentation files present (README.md, CLAUDE.md, AGENTS.md)"
fi

# Test 7: docs/ directory exists
echo "Test: docs/ directory exists"
if [ -d "${MODULE_DIR}/docs" ]; then
    pass "docs/ directory exists"
else
    fail "docs/ directory missing"
fi

# Test 8: All 5 packages compile independently
echo "Test: All packages compile independently"
all_compile=true
for pkg in client qdrant pinecone milvus pgvector; do
    if [ -d "${MODULE_DIR}/pkg/${pkg}" ]; then
        if ! (cd "${MODULE_DIR}" && go build "./pkg/${pkg}/..." 2>/dev/null); then
            fail "Package pkg/${pkg} failed to compile"
            all_compile=false
        fi
    fi
done
if [ "$all_compile" = true ]; then
    pass "All packages compile independently"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
