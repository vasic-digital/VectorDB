// CONST-035 / Article XI §11.9 call-site coverage for the i18n
// migration of pkg/client Validate methods. Tests assert that:
//   - default NoopTranslator emits canonical key as the error message;
//   - SetTranslator(t) rewires every Validate so a fake Translator's
//     output is observed end-to-end (paired-mutation: planting a
//     fakeTranslator that returns "translated:KEY" MUST change the
//     observed error text — proves the seam is wired, not stubbed).
//
// CONST-051(B): the test uses only pkg/client + pkg/i18n; no parent-
// project imports, so the submodule remains project-not-aware.
package client_test

import (
	"strings"
	"sync"
	"testing"

	"digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/i18n"
)

// fakeTranslator is a unit-test-only double permitted under
// CONST-050(A). It returns "translated:KEY" so call-site assertions
// can distinguish translator-rendered output from the legacy English
// fragment.
type fakeTranslator struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeTranslator) T(key string, _ map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, key)
	return "translated:" + key
}

func (f *fakeTranslator) seen(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == key {
			return true
		}
	}
	return false
}

func TestValidate_NoopTranslator_EmitsKeyVerbatim(t *testing.T) {
	// Make sure the package starts on the NoopTranslator default.
	client.SetTranslator(nil)
	t.Cleanup(func() { client.SetTranslator(nil) })

	cases := []struct {
		name string
		err  error
		key  string
	}{
		{
			name: "vector_empty",
			err:  (&client.Vector{Values: nil}).Validate(),
			key:  "vectordb_validation_vector_values_empty",
		},
		{
			name: "query_vector_empty",
			err:  (&client.SearchQuery{Vector: nil, TopK: 5}).Validate(),
			key:  "vectordb_validation_query_vector_empty",
		},
		{
			name: "topk_nonpositive",
			err:  (&client.SearchQuery{Vector: []float32{0.1}, TopK: 0}).Validate(),
			key:  "vectordb_validation_topk_positive",
		},
		{
			name: "collection_name_required",
			err:  (&client.CollectionConfig{Dimension: 8}).Validate(),
			key:  "vectordb_validation_collection_name_required",
		},
		{
			name: "dimension_min",
			err:  (&client.CollectionConfig{Name: "c", Dimension: 0}).Validate(),
			key:  "vectordb_validation_dimension_min",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if c.err.Error() != c.key {
				t.Fatalf("Validate error = %q, want bare key %q (NoopTranslator)", c.err.Error(), c.key)
			}
		})
	}
}

func TestValidate_FakeTranslator_RewiresRendering(t *testing.T) {
	fake := &fakeTranslator{}
	client.SetTranslator(fake)
	t.Cleanup(func() { client.SetTranslator(nil) })

	cases := []struct {
		name string
		err  error
		key  string
	}{
		{
			name: "vector_empty",
			err:  (&client.Vector{}).Validate(),
			key:  "vectordb_validation_vector_values_empty",
		},
		{
			name: "query_vector_empty",
			err:  (&client.SearchQuery{Vector: nil, TopK: 5}).Validate(),
			key:  "vectordb_validation_query_vector_empty",
		},
		{
			name: "topk_nonpositive",
			err:  (&client.SearchQuery{Vector: []float32{0.1, 0.2}, TopK: -1}).Validate(),
			key:  "vectordb_validation_topk_positive",
		},
		{
			name: "collection_name_required",
			err:  (&client.CollectionConfig{Dimension: 16}).Validate(),
			key:  "vectordb_validation_collection_name_required",
		},
		{
			name: "dimension_min",
			err:  (&client.CollectionConfig{Name: "c", Dimension: -1}).Validate(),
			key:  "vectordb_validation_dimension_min",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			want := "translated:" + c.key
			if c.err.Error() != want {
				t.Fatalf("Validate error = %q, want %q (translator must be invoked)", c.err.Error(), want)
			}
			if !fake.seen(c.key) {
				t.Fatalf("translator was not asked for key %q", c.key)
			}
		})
	}
}

func TestSetTranslator_NilRestoresNoop(t *testing.T) {
	fake := &fakeTranslator{}
	client.SetTranslator(fake)
	if _, ok := client.Translator().(*fakeTranslator); !ok {
		t.Fatalf("SetTranslator(fake) did not install fake; got %T", client.Translator())
	}

	client.SetTranslator(nil)
	if _, ok := client.Translator().(i18n.NoopTranslator); !ok {
		t.Fatalf("SetTranslator(nil) did not restore NoopTranslator; got %T", client.Translator())
	}
}

// TestValidate_ParametricMutationGuard is the §11.4.32 mutation-paired
// check: it asserts that swapping the package translator changes the
// observable Validate output for EVERY migrated key. If a future edit
// accidentally hardcodes an English fragment back, the fake's
// "translated:" prefix will be missing and this test fails.
func TestValidate_ParametricMutationGuard(t *testing.T) {
	fake := &fakeTranslator{}
	client.SetTranslator(fake)
	t.Cleanup(func() { client.SetTranslator(nil) })

	probes := []func() error{
		func() error { return (&client.Vector{}).Validate() },
		func() error { return (&client.SearchQuery{TopK: 1}).Validate() },
		func() error { return (&client.SearchQuery{Vector: []float32{0.1}, TopK: 0}).Validate() },
		func() error { return (&client.CollectionConfig{Dimension: 8}).Validate() },
		func() error { return (&client.CollectionConfig{Name: "c"}).Validate() },
	}

	for i, probe := range probes {
		err := probe()
		if err == nil {
			t.Fatalf("probe %d returned nil; expected validation error", i)
		}
		if !strings.HasPrefix(err.Error(), "translated:") {
			t.Fatalf("probe %d emitted %q; expected translator-prefixed output (call site bypassed translator)", i, err.Error())
		}
	}
}
