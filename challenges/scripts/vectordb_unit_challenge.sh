#!/usr/bin/env bash
# vectordb_unit_challenge.sh - Validates VectorDB module unit tests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="VectorDB"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== ${MODULE_NAME} Unit Test Challenge ==="
echo ""

# Resource limits per project constitution
export GOMAXPROCS=2

# Test 1: Test files exist in pkg/
echo "Test: Test files exist"
test_count=$(find "${MODULE_DIR}/pkg" -name "*_test.go" -type f 2>/dev/null | wc -l)
if [ "$test_count" -gt 0 ]; then
    pass "Found ${test_count} test files in pkg/"
else
    fail "No test files found in pkg/"
fi

# Test 2: Unit tests pass
echo "Test: Unit tests pass (GOMAXPROCS=2, -short, -count=1, -p 1)"
TEST_OUTPUT=$(mktemp)
if (cd "${MODULE_DIR}" && nice -n 19 ionice -c 3 go test -short -count=1 -p 1 ./pkg/... > "${TEST_OUTPUT}" 2>&1); then
    pass "All unit tests pass"
else
    fail "Unit tests failed"
    tail -20 "${TEST_OUTPUT}"
fi

# Test 3: No FAIL lines in output
echo "Test: No FAIL lines in test output"
if grep -q "^FAIL" "${TEST_OUTPUT}" 2>/dev/null; then
    fail "FAIL lines detected in test output"
    grep "^FAIL" "${TEST_OUTPUT}"
else
    pass "No FAIL lines in test output"
fi
rm -f "${TEST_OUTPUT}"

# Test 4-8: Each package has tests
for pkg in client qdrant pinecone milvus pgvector; do
    echo "Test: Package pkg/${pkg} has test files"
    if ls "${MODULE_DIR}/pkg/${pkg}/"*_test.go >/dev/null 2>&1; then
        pass "Package pkg/${pkg} has test files"
    else
        fail "Package pkg/${pkg} missing test files"
    fi
done

# Test 9: Tests also exist in tests/ directory
echo "Test: tests/ directory has test files"
if find "${MODULE_DIR}/tests" -name "*_test.go" -type f 2>/dev/null | grep -q .; then
    pass "tests/ directory has test files"
else
    fail "tests/ directory missing test files"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
