#!/usr/bin/env bash
# vectordb_functionality_challenge.sh - Validates VectorDB module core functionality
# Checks similarity search interfaces, collection management, and vector store adapters
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="VectorDB"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Functionality Challenge ==="
echo ""

# --- Section 1: Required packages ---
echo "Section 1: Required packages (5)"

for pkg in client qdrant pinecone milvus pgvector; do
    echo "Test: Package pkg/${pkg} exists"
    if [ -d "${MODULE_DIR}/pkg/${pkg}" ]; then
        pass "Package pkg/${pkg} exists"
    else
        fail "Package pkg/${pkg} missing"
    fi
done

# --- Section 2: Core interfaces ---
echo ""
echo "Section 2: Core vector store interfaces"

echo "Test: VectorStore interface exists"
if grep -q "type VectorStore interface" "${MODULE_DIR}/pkg/client/"*.go 2>/dev/null; then
    pass "VectorStore interface exists"
else
    fail "VectorStore interface missing"
fi

echo "Test: CollectionManager interface exists"
if grep -q "type CollectionManager interface" "${MODULE_DIR}/pkg/client/"*.go 2>/dev/null; then
    pass "CollectionManager interface exists"
else
    fail "CollectionManager interface missing"
fi

echo "Test: Vector struct exists"
if grep -q "type Vector struct" "${MODULE_DIR}/pkg/client/"*.go 2>/dev/null; then
    pass "Vector struct exists"
else
    fail "Vector struct missing"
fi

echo "Test: SearchResult struct exists"
if grep -q "type SearchResult struct" "${MODULE_DIR}/pkg/client/"*.go 2>/dev/null; then
    pass "SearchResult struct exists"
else
    fail "SearchResult struct missing"
fi

echo "Test: SearchQuery struct exists"
if grep -q "type SearchQuery struct" "${MODULE_DIR}/pkg/client/"*.go 2>/dev/null; then
    pass "SearchQuery struct exists"
else
    fail "SearchQuery struct missing"
fi

echo "Test: CollectionConfig struct exists"
if grep -q "type CollectionConfig struct" "${MODULE_DIR}/pkg/client/"*.go 2>/dev/null; then
    pass "CollectionConfig struct exists"
else
    fail "CollectionConfig struct missing"
fi

# --- Section 3: Qdrant adapter ---
echo ""
echo "Section 3: Qdrant adapter"

echo "Test: Qdrant Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/qdrant/"*.go 2>/dev/null; then
    pass "Qdrant Client struct exists"
else
    fail "Qdrant Client struct missing"
fi

echo "Test: Qdrant Config struct exists"
if grep -q "type Config struct" "${MODULE_DIR}/pkg/qdrant/"*.go 2>/dev/null; then
    pass "Qdrant Config struct exists"
else
    fail "Qdrant Config struct missing"
fi

# --- Section 4: Pinecone adapter ---
echo ""
echo "Section 4: Pinecone adapter"

echo "Test: Pinecone Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/pinecone/"*.go 2>/dev/null; then
    pass "Pinecone Client struct exists"
else
    fail "Pinecone Client struct missing"
fi

echo "Test: Pinecone Config struct exists"
if grep -q "type Config struct" "${MODULE_DIR}/pkg/pinecone/"*.go 2>/dev/null; then
    pass "Pinecone Config struct exists"
else
    fail "Pinecone Config struct missing"
fi

# --- Section 5: Milvus adapter ---
echo ""
echo "Section 5: Milvus adapter"

echo "Test: Milvus Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/milvus/"*.go 2>/dev/null; then
    pass "Milvus Client struct exists"
else
    fail "Milvus Client struct missing"
fi

echo "Test: Milvus Config struct exists"
if grep -q "type Config struct" "${MODULE_DIR}/pkg/milvus/"*.go 2>/dev/null; then
    pass "Milvus Config struct exists"
else
    fail "Milvus Config struct missing"
fi

# --- Section 6: pgvector adapter ---
echo ""
echo "Section 6: pgvector adapter"

echo "Test: pgvector Client struct exists"
if grep -q "type Client struct" "${MODULE_DIR}/pkg/pgvector/"*.go 2>/dev/null; then
    pass "pgvector Client struct exists"
else
    fail "pgvector Client struct missing"
fi

echo "Test: pgvector DBPool interface exists"
if grep -q "type DBPool interface" "${MODULE_DIR}/pkg/pgvector/"*.go 2>/dev/null; then
    pass "pgvector DBPool interface exists"
else
    fail "pgvector DBPool interface missing"
fi

# --- Section 7: Source structure completeness ---
echo ""
echo "Section 7: Source structure"

echo "Test: Each package has non-test Go source files"
all_have_source=true
for pkg in client qdrant pinecone milvus pgvector; do
    non_test=$(find "${MODULE_DIR}/pkg/${pkg}" -name "*.go" ! -name "*_test.go" -type f 2>/dev/null | wc -l)
    if [ "$non_test" -eq 0 ]; then
        fail "Package pkg/${pkg} has no non-test Go files"
        all_have_source=false
    fi
done
if [ "$all_have_source" = true ]; then
    pass "All packages have non-test Go source files"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
