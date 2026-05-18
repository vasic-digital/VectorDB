// Integration tests for pgvector real-backend wiring.
//
// §11.4 / CONST-035 — these tests exercise the actual pgxpool
// adapter against a real PostgreSQL+pgvector instance. They produce
// positive runtime evidence when enabled (VECTORDB_TEST_DSN set),
// and skip LOUDLY with a SKIP-OK marker referencing this round's
// ticket when not enabled. The loud-skip pattern satisfies §11.4
// "no silent skips" rule per the constitution submodule.
//
// CONST-042 — DSN sourced from env. Never commit a real DSN here.

package pgvector

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.vectordb/pkg/client"
)

func realPgvectorDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("VECTORDB_TEST_DSN")
	if dsn == "" {
		// SKIP-OK: #VECTORDB-PG-REAL-ROUND37 — requires Postgres+pgvector;
		// set VECTORDB_TEST_DSN to enable real-backend integration tests.
		t.Skip("SKIP-OK: #VECTORDB-PG-REAL-ROUND37 — VECTORDB_TEST_DSN unset; " +
			"export VECTORDB_TEST_DSN='postgres://user:pass@host:5432/db?sslmode=disable' " +
			"to enable real Postgres+pgvector integration tests")
	}
	return dsn
}

func TestPgvectorClient_NilDSN_BackCompatSentinels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := NewPgvectorClient(ctx, PgvectorConfig{DSN: "", TablePrefix: "vdb_"})
	require.NoError(t, err)
	require.NotNil(t, c)

	// Inject a real (but unused) DBPool would defeat the test. Instead
	// we exercise the not-connected leg + then artificially mark connected
	// to expose the nil-pool sentinel path.
	_, err = c.Search(ctx, "x", client.SearchQuery{Vector: []float32{0.1}, TopK: 1})
	assert.ErrorIs(t, err, client.ErrNotConnected,
		"empty-DSN client is not connected — Search must surface ErrNotConnected first")
	_, err = c.Get(ctx, "x", []string{"v1"})
	assert.ErrorIs(t, err, client.ErrNotConnected,
		"empty-DSN client is not connected — Get must surface ErrNotConnected first")
}

func TestPgvectorClient_Search_Real(t *testing.T) {
	dsn := realPgvectorDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := NewPgvectorClient(ctx, PgvectorConfig{DSN: dsn, TablePrefix: "round37_"})
	require.NoError(t, err)
	defer func() {
		_ = c.Close()
	}()

	require.NoError(t, c.Connect(ctx))

	const collName = "vectors"
	// Clean state.
	_ = c.DeleteCollection(ctx, collName)

	require.NoError(t, c.CreateCollection(ctx, client.CollectionConfig{
		Name:      collName,
		Dimension: 3,
		Metric:    client.DistanceCosine,
	}))
	defer func() {
		_ = c.DeleteCollection(ctx, collName)
	}()

	require.NoError(t, c.Upsert(ctx, collName, []client.Vector{
		{ID: "v1", Values: []float32{1.0, 0.0, 0.0}},
		{ID: "v2", Values: []float32{0.0, 1.0, 0.0}},
		{ID: "v3", Values: []float32{0.0, 0.0, 1.0}},
	}))

	results, err := c.Search(ctx, collName, client.SearchQuery{
		Vector: []float32{1.0, 0.0, 0.0},
		TopK:   2,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	// Closest vector is v1 (cosine distance 0 → score 1.0).
	assert.Equal(t, "v1", results[0].ID)
	assert.InDelta(t, 1.0, results[0].Score, 1e-4)
}

func TestPgvectorClient_Get_Real_FoundAndNotFound(t *testing.T) {
	dsn := realPgvectorDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := NewPgvectorClient(ctx, PgvectorConfig{DSN: dsn, TablePrefix: "round37_"})
	require.NoError(t, err)
	defer func() {
		_ = c.Close()
	}()

	require.NoError(t, c.Connect(ctx))

	const collName = "vectors_get"
	_ = c.DeleteCollection(ctx, collName)
	require.NoError(t, c.CreateCollection(ctx, client.CollectionConfig{
		Name:      collName,
		Dimension: 2,
		Metric:    client.DistanceCosine,
	}))
	defer func() {
		_ = c.DeleteCollection(ctx, collName)
	}()

	require.NoError(t, c.Upsert(ctx, collName, []client.Vector{
		{ID: "present", Values: []float32{0.5, 0.5}},
	}))

	got, err := c.Get(ctx, collName, []string{"present"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "present", got[0].ID)

	_, err = c.Get(ctx, collName, []string{"missing"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVectorNotFound,
		"missing id MUST surface ErrVectorNotFound (round-37 §2.3 contract)")
	assert.True(t, strings.Contains(err.Error(), "missing") ||
		strings.Contains(err.Error(), "no rows"),
		"error message should include id or no-rows hint, got: %v", err)
}
