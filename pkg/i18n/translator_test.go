// CONST-035 / Article XI §11.9 unit coverage for the VectorDB i18n
// translator seam. Tests assert observable behaviour (string equality on
// returned keys, behaviour invariance across nil/empty params) rather
// than presence of constructors.
package i18n_test

import (
	"sync"
	"testing"

	"digital.vasic.vectordb/pkg/i18n"
)

func TestNoopTranslator_ReturnsKeyVerbatim(t *testing.T) {
	tr := i18n.NoopTranslator{}

	cases := []struct {
		key string
	}{
		{"vectordb_validation_vector_values_empty"},
		{"vectordb_validation_query_vector_empty"},
		{"vectordb_validation_topk_positive"},
		{"vectordb_validation_collection_name_required"},
		{"vectordb_validation_dimension_min"},
		{""},
		{"unknown_key_with_dots.and.colons:plus-dashes"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.key, func(t *testing.T) {
			got := tr.T(c.key, nil)
			if got != c.key {
				t.Fatalf("NoopTranslator.T(%q, nil) = %q, want key verbatim", c.key, got)
			}
		})
	}
}

func TestNoopTranslator_IgnoresParams(t *testing.T) {
	tr := i18n.NoopTranslator{}
	key := "vectordb_validation_topk_positive"
	params := map[string]any{
		"requested": 0,
		"minimum":   1,
	}

	got := tr.T(key, params)
	if got != key {
		t.Fatalf("NoopTranslator.T(%q, params) = %q, want %q (params must be ignored)", key, got, key)
	}
}

func TestNoopTranslator_ConcurrentSafe(t *testing.T) {
	tr := i18n.NoopTranslator{}
	const goroutines = 64
	const iterations = 256

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if got := tr.T("vectordb_validation_dimension_min", nil); got != "vectordb_validation_dimension_min" {
					t.Errorf("concurrent T() returned %q", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// fakeTranslator is a unit-test-only test double — permitted under
// CONST-050(A) because this file is *_test.go. It proves the Translator
// interface is satisfiable by a non-Noop implementation and gives the
// call-site tests a way to verify that SetTranslator actually rewires
// rendering.
type fakeTranslator struct {
	calls map[string]int
	mu    sync.Mutex
}

func (f *fakeTranslator) T(key string, _ map[string]any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[key]++
	return "translated:" + key
}

func TestTranslator_InterfaceSatisfaction(t *testing.T) {
	var _ i18n.Translator = i18n.NoopTranslator{}
	var _ i18n.Translator = &fakeTranslator{}
}
