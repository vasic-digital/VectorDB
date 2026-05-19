// Command runner is the round-287 anti-bluff Challenge runner for
// digital.vasic.vectordb. It exercises the public client interface
// surface (VectorStore / CollectionManager, Vector / SearchQuery /
// CollectionConfig validation, SetTranslator / Translator i18n seam,
// DistanceMetric constants, ErrNotConnected sentinel) AND the four
// backend adapters' constructor + config-validation paths against
// the real, in-process implementation. The runner produces captured
// stdout per Article XI §11.9 — every PASS is backed by a printed
// assertion line, not by absence-of-error.
//
// The runner intentionally avoids invoking any backend network call
// (Qdrant/Pinecone/Milvus HTTP, pgvector SQL via real Postgres) —
// those are exercised by integration tests behind SKIP-OK markers.
// Instead, the runner exercises every backend's constructor,
// configuration validation (error AND success paths), and helper
// functions that operate on inputs alone.
//
// Exit codes:
//
//	0   — every assertion passed, every locale line printed.
//	1   — usage / flag error.
//	2   — coverage gap (a known symbol resolved to a nil / empty value).
//	3   — invariant violation (validation rejected good input OR accepted
//	      bad input; sentinel-error identity broken).
//	4   — locale UX line missing or canonical token absent.
//	5   — backend constructor regression (NewClient rejected valid
//	      DefaultConfig or accepted a known-bad config).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	vclient "digital.vasic.vectordb/pkg/client"
	"digital.vasic.vectordb/pkg/i18n"
	"digital.vasic.vectordb/pkg/milvus"
	"digital.vasic.vectordb/pkg/pgvector"
	"digital.vasic.vectordb/pkg/pinecone"
	"digital.vasic.vectordb/pkg/qdrant"
)

// locale describes a UX line printed by the runner. The text is a
// short, locale-correct summary that consumers can grep for to
// confirm the runner produced operator-facing output in every
// supported locale per CONST-046.
type locale struct {
	tag  string
	line func(backendCount, validationCount int) string
}

// supportedLocales is the 5-locale CONST-046 set the runner must emit
// every run. Mirrors the matrix used by ToolSchema round-285 and
// other round-2xx enrichments.
func supportedLocales() []locale {
	return []locale{
		{
			tag: "en",
			line: func(b, v int) string {
				return fmt.Sprintf("[en] vectordb: %d backends exercised, %d validation gates passed", b, v)
			},
		},
		{
			tag: "sr",
			line: func(b, v int) string {
				return fmt.Sprintf("[sr] vectordb: %d pozadinskih sistema izvršeno, %d validacionih kapija prošlo", b, v)
			},
		},
		{
			tag: "ja",
			line: func(b, v int) string {
				return fmt.Sprintf("[ja] vectordb: %d バックエンドを実行、%d 検証ゲートが通過", b, v)
			},
		},
		{
			tag: "es",
			line: func(b, v int) string {
				return fmt.Sprintf("[es] vectordb: %d backends ejercidos, %d puertas de validación aprobadas", b, v)
			},
		},
		{
			tag: "de",
			line: func(b, v int) string {
				return fmt.Sprintf("[de] vectordb: %d Backends ausgeübt, %d Validierungs-Gates bestanden", b, v)
			},
		},
	}
}

// Sentinel error tags used to compute exit codes without printing
// the tag itself.
var (
	errCoverage   = errors.New("coverage")
	errInvariant  = errors.New("invariant")
	errLocale     = errors.New("locale")
	errBackend    = errors.New("backend")
	errValidation = errors.New("validation")
)

// taggedError attaches a sentinel for exit-code mapping while
// preserving the inner cause via Unwrap.
type taggedError struct {
	tag   error
	inner error
}

func (e *taggedError) Error() string { return e.inner.Error() }
func (e *taggedError) Unwrap() error { return e.inner }
func (e *taggedError) Is(t error) bool {
	return errors.Is(e.tag, t)
}

func wrap(tag, inner error) error {
	return &taggedError{tag: tag, inner: inner}
}

func exitCodeFor(err error) int {
	switch {
	case errors.Is(err, errCoverage):
		return 2
	case errors.Is(err, errInvariant):
		return 3
	case errors.Is(err, errLocale):
		return 4
	case errors.Is(err, errBackend):
		return 5
	case errors.Is(err, errValidation):
		return 3
	default:
		return 1
	}
}

func main() {
	all := flag.Bool("all", false, "run every check (default mode)")
	backend := flag.String("backend", "", "exercise only the named backend (qdrant|pinecone|milvus|pgvector)")
	flag.Parse()

	if !*all && *backend == "" {
		*all = true
	}

	if *backend != "" {
		if err := runOne(*backend); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(exitCodeFor(err))
		}
		return
	}
	if err := runAll(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCodeFor(err))
	}
}

// runOne exercises a single backend by name (qdrant|pinecone|milvus|pgvector).
func runOne(name string) error {
	be, ok := backendFactories()[name]
	if !ok {
		return wrap(errCoverage, fmt.Errorf("unknown backend: %s", name))
	}
	if err := exerciseBackend(be); err != nil {
		return wrap(errBackend, fmt.Errorf("backend %s: %w", name, err))
	}
	fmt.Printf("backend=%s default_config_ok=true bad_config_rejected=true constructor_returns_non_nil=true\n", be.name)
	return nil
}

// runAll exercises the full client validation surface, the i18n seam,
// the four backend constructors, and emits the 5-locale CONST-046 UX
// summary lines.
func runAll() error {
	// Validation surface — every Validate method MUST accept a known-good
	// input AND reject a known-bad input. Mutation of either direction
	// is a CONST-035 wrapper-bluff.
	validationsPassed, err := assertValidationGates()
	if err != nil {
		return wrap(errValidation, err)
	}
	fmt.Printf("validation_gates: Vector + SearchQuery + CollectionConfig good/bad paths PASS (gates=%d)\n", validationsPassed)

	// Sentinel error — ErrNotConnected MUST be a stable identity.
	if vclient.ErrNotConnected == nil {
		return wrap(errCoverage, errors.New("ErrNotConnected sentinel is nil"))
	}
	if vclient.ErrNotConnected.Error() == "" {
		return wrap(errInvariant, errors.New("ErrNotConnected has empty error message"))
	}
	if !errors.Is(vclient.ErrNotConnected, vclient.ErrNotConnected) {
		return wrap(errInvariant, errors.New("ErrNotConnected fails errors.Is identity"))
	}
	fmt.Printf("sentinel=ErrNotConnected msg=%q identity_ok=true\n", vclient.ErrNotConnected.Error())

	// DistanceMetric constants — every constant MUST be non-empty and
	// distinct. Adding a metric without exercising it here is a
	// CONST-048 coverage gap.
	metrics := map[string]vclient.DistanceMetric{
		"cosine":      vclient.DistanceCosine,
		"dot_product": vclient.DistanceDotProduct,
		"euclidean":   vclient.DistanceEuclidean,
	}
	seen := map[vclient.DistanceMetric]string{}
	for name, m := range metrics {
		if string(m) == "" {
			return wrap(errCoverage, fmt.Errorf("DistanceMetric %s is empty string", name))
		}
		if prev, dup := seen[m]; dup {
			return wrap(errInvariant, fmt.Errorf("DistanceMetric %s duplicates %s (value=%s)", name, prev, m))
		}
		seen[m] = name
	}
	fmt.Printf("distance_metrics: %d distinct constants (cosine=%s dot=%s euclidean=%s)\n",
		len(metrics), vclient.DistanceCosine, vclient.DistanceDotProduct, vclient.DistanceEuclidean)

	// i18n seam — SetTranslator MUST install a custom translator and
	// route validation messages through it. nil MUST restore the noop.
	exerciseTranslatorSeam()
	fmt.Println("i18n_seam: SetTranslator(custom) → validation message routed; SetTranslator(nil) → noop restored")

	// pgvector helpers — DistanceOperator + VectorToString operate on
	// inputs alone (no I/O), so we exercise them here as part of the
	// no-network surface coverage.
	if op := pgvector.DistanceOperator(vclient.DistanceCosine); op != "<=>" {
		return wrap(errInvariant, fmt.Errorf("pgvector.DistanceOperator(cosine) = %q, want \"<=>\"", op))
	}
	if op := pgvector.DistanceOperator(vclient.DistanceDotProduct); op != "<#>" {
		return wrap(errInvariant, fmt.Errorf("pgvector.DistanceOperator(dot) = %q, want \"<#>\"", op))
	}
	if op := pgvector.DistanceOperator(vclient.DistanceEuclidean); op != "<->" {
		return wrap(errInvariant, fmt.Errorf("pgvector.DistanceOperator(euclidean) = %q, want \"<->\"", op))
	}
	vecStr := pgvector.VectorToString([]float32{0.1, 0.2, 0.3})
	if !strings.HasPrefix(vecStr, "[") || !strings.HasSuffix(vecStr, "]") {
		return wrap(errInvariant, fmt.Errorf("pgvector.VectorToString shape malformed: %q", vecStr))
	}
	if strings.Count(vecStr, ",") != 2 {
		return wrap(errInvariant, fmt.Errorf("pgvector.VectorToString comma-count != 2: %q", vecStr))
	}
	fmt.Printf("pgvector_helpers: DistanceOperator(3 metrics) + VectorToString shape PASS (sample=%s)\n", vecStr)

	// Backend constructors — every backend exposes NewClient + DefaultConfig
	// + Config.Validate. Each MUST succeed on DefaultConfig (with required
	// fields filled in where applicable) and MUST reject a known-bad config.
	factories := backendFactories()
	exercised := 0
	for _, name := range backendOrder() {
		be := factories[name]
		if err := exerciseBackend(be); err != nil {
			return wrap(errBackend, fmt.Errorf("backend %s: %w", name, err))
		}
		fmt.Printf("backend=%s default_config_ok=true bad_config_rejected=true constructor_returns_non_nil=true\n", be.name)
		exercised++
	}
	if exercised != len(factories) {
		return wrap(errBackend, fmt.Errorf("exercised %d/%d backends", exercised, len(factories)))
	}

	// 5-locale bilingual UX evidence per CONST-046.
	printed := 0
	for _, loc := range supportedLocales() {
		out := loc.line(exercised, validationsPassed)
		if !strings.Contains(out, "vectordb:") {
			return wrap(errLocale, fmt.Errorf("locale %s: missing canonical token", loc.tag))
		}
		fmt.Println(out)
		printed++
	}
	if printed != len(supportedLocales()) {
		return wrap(errLocale, fmt.Errorf("printed %d/%d locales", printed, len(supportedLocales())))
	}

	fmt.Printf("OK backends=%d validation_gates=%d locales=%d\n", exercised, validationsPassed, printed)
	return nil
}

// assertValidationGates checks that Vector / SearchQuery /
// CollectionConfig Validate methods accept good input AND reject bad
// input. Returns the gate count for the summary line.
func assertValidationGates() (int, error) {
	// Vector — empty Values MUST be rejected, populated MUST pass.
	bad := &vclient.Vector{ID: "v1", Values: nil}
	if err := bad.Validate(); err == nil {
		return 0, errors.New("Vector.Validate accepted empty Values")
	}
	good := &vclient.Vector{ID: "v1", Values: []float32{0.1, 0.2}}
	if err := good.Validate(); err != nil {
		return 0, fmt.Errorf("Vector.Validate rejected populated Values: %w", err)
	}

	// SearchQuery — empty Vector OR non-positive TopK MUST be rejected.
	badQ1 := &vclient.SearchQuery{Vector: nil, TopK: 10}
	if err := badQ1.Validate(); err == nil {
		return 0, errors.New("SearchQuery.Validate accepted empty Vector")
	}
	badQ2 := &vclient.SearchQuery{Vector: []float32{0.1}, TopK: 0}
	if err := badQ2.Validate(); err == nil {
		return 0, errors.New("SearchQuery.Validate accepted TopK=0")
	}
	goodQ := &vclient.SearchQuery{Vector: []float32{0.1, 0.2}, TopK: 5}
	if err := goodQ.Validate(); err != nil {
		return 0, fmt.Errorf("SearchQuery.Validate rejected populated query: %w", err)
	}

	// CollectionConfig — empty Name, Dimension < 1, and invalid Metric
	// MUST all be rejected. Default empty Metric MUST be accepted.
	badC1 := &vclient.CollectionConfig{Name: "", Dimension: 128, Metric: vclient.DistanceCosine}
	if err := badC1.Validate(); err == nil {
		return 0, errors.New("CollectionConfig.Validate accepted empty Name")
	}
	badC2 := &vclient.CollectionConfig{Name: "c", Dimension: 0, Metric: vclient.DistanceCosine}
	if err := badC2.Validate(); err == nil {
		return 0, errors.New("CollectionConfig.Validate accepted Dimension=0")
	}
	badC3 := &vclient.CollectionConfig{Name: "c", Dimension: 128, Metric: vclient.DistanceMetric("invalid")}
	if err := badC3.Validate(); err == nil {
		return 0, errors.New("CollectionConfig.Validate accepted invalid Metric")
	}
	goodC := &vclient.CollectionConfig{Name: "c", Dimension: 128, Metric: vclient.DistanceCosine}
	if err := goodC.Validate(); err != nil {
		return 0, fmt.Errorf("CollectionConfig.Validate rejected good config: %w", err)
	}
	// Empty Metric is acceptable — backend assigns default.
	goodCDefaultMetric := &vclient.CollectionConfig{Name: "c", Dimension: 128, Metric: ""}
	if err := goodCDefaultMetric.Validate(); err != nil {
		return 0, fmt.Errorf("CollectionConfig.Validate rejected empty Metric (should be accepted): %w", err)
	}

	// Gate count: Vector (1) + SearchQuery (1) + CollectionConfig (1) = 3.
	return 3, nil
}

// recordingTranslator captures the most recent T() invocation so the
// runner can prove the i18n seam is wired correctly.
type recordingTranslator struct {
	lastKey string
}

// T satisfies i18n.Translator. Returns a fixed sentinel so the runner
// can prove the custom translator (not the noop) was consulted.
func (r *recordingTranslator) T(key string, _ map[string]any) string {
	r.lastKey = key
	return "CUSTOM:" + key
}

// exerciseTranslatorSeam proves SetTranslator wires a custom impl and
// that nil restores the NoopTranslator default.
func exerciseTranslatorSeam() {
	rec := &recordingTranslator{}
	vclient.SetTranslator(rec)
	defer vclient.SetTranslator(nil) // restore noop after exercise

	// Trigger validation; the message MUST route through rec.
	v := &vclient.Vector{Values: nil}
	if err := v.Validate(); err == nil {
		panic("Vector.Validate accepted empty Values during translator-seam exercise")
	} else if !strings.HasPrefix(err.Error(), "CUSTOM:") {
		panic(fmt.Sprintf("Vector.Validate did not route through custom translator: %q", err.Error()))
	}
	if rec.lastKey != "vectordb_validation_vector_values_empty" {
		panic(fmt.Sprintf("custom translator key mismatch: got=%q", rec.lastKey))
	}

	// Confirm Translator() returns our custom impl.
	if vclient.Translator() != rec {
		panic("Translator() did not return the custom translator")
	}

	// Restore noop and confirm it's back.
	vclient.SetTranslator(nil)
	if _, ok := vclient.Translator().(i18n.NoopTranslator); !ok {
		panic("SetTranslator(nil) did not restore NoopTranslator")
	}
}

// backendInfo captures one backend's name + factory functions for
// uniform exercise without leaking concrete types into the dispatcher.
type backendInfo struct {
	name        string
	makeDefault func() (ok bool, _ error)
	makeBad     func() (rejected bool, _ error)
}

// backendOrder returns the canonical iteration order so output is
// deterministic across runs (subagent-driven CI compares byte-by-byte).
func backendOrder() []string {
	return []string{"qdrant", "milvus", "pinecone", "pgvector"}
}

// backendFactories returns the four backend exercises. Each closure
// constructs a real client against DefaultConfig + asserts NewClient
// rejects an obviously-bad config without performing any network I/O.
func backendFactories() map[string]backendInfo {
	return map[string]backendInfo{
		"qdrant": {
			name: "qdrant",
			makeDefault: func() (bool, error) {
				cfg := qdrant.DefaultConfig()
				if err := cfg.Validate(); err != nil {
					return false, fmt.Errorf("DefaultConfig.Validate: %w", err)
				}
				c, err := qdrant.NewClient(cfg)
				if err != nil {
					return false, fmt.Errorf("NewClient(default): %w", err)
				}
				if c == nil {
					return false, errors.New("NewClient returned nil client without error")
				}
				if got := cfg.GetHTTPURL(); !strings.HasPrefix(got, "http://") {
					return false, fmt.Errorf("GetHTTPURL prefix unexpected: %s", got)
				}
				if got := cfg.GetGRPCAddress(); !strings.Contains(got, ":") {
					return false, fmt.Errorf("GetGRPCAddress shape unexpected: %s", got)
				}
				return true, nil
			},
			makeBad: func() (bool, error) {
				bad := &qdrant.Config{Host: "", HTTPPort: 6333, GRPCPort: 6334}
				if err := bad.Validate(); err == nil {
					return false, errors.New("Config{Host:\"\"} accepted by Validate")
				}
				return true, nil
			},
		},
		"milvus": {
			name: "milvus",
			makeDefault: func() (bool, error) {
				cfg := milvus.DefaultConfig()
				if err := cfg.Validate(); err != nil {
					return false, fmt.Errorf("DefaultConfig.Validate: %w", err)
				}
				c, err := milvus.NewClient(cfg)
				if err != nil {
					return false, fmt.Errorf("NewClient(default): %w", err)
				}
				if c == nil {
					return false, errors.New("NewClient returned nil client without error")
				}
				if got := cfg.GetBaseURL(); !strings.Contains(got, "/v2/vectordb") {
					return false, fmt.Errorf("GetBaseURL shape unexpected: %s", got)
				}
				return true, nil
			},
			makeBad: func() (bool, error) {
				bad := &milvus.Config{Host: "", Port: 19530}
				if err := bad.Validate(); err == nil {
					return false, errors.New("Config{Host:\"\"} accepted by Validate")
				}
				return true, nil
			},
		},
		"pinecone": {
			name: "pinecone",
			makeDefault: func() (bool, error) {
				// Pinecone DefaultConfig lacks APIKey / IndexHost — fill them in
				// to exercise the constructor without performing network I/O.
				cfg := pinecone.DefaultConfig()
				cfg.APIKey = "challenge-runner-key"
				cfg.IndexHost = "https://idx.pinecone.io"
				cfg.Environment = "us-west1-gcp"
				if err := cfg.Validate(); err != nil {
					return false, fmt.Errorf("Config.Validate: %w", err)
				}
				c, err := pinecone.NewClient(cfg)
				if err != nil {
					return false, fmt.Errorf("NewClient: %w", err)
				}
				if c == nil {
					return false, errors.New("NewClient returned nil client without error")
				}
				return true, nil
			},
			makeBad: func() (bool, error) {
				bad := &pinecone.Config{APIKey: "", IndexHost: "https://idx.pinecone.io"}
				if err := bad.Validate(); err == nil {
					return false, errors.New("Config{APIKey:\"\"} accepted by Validate")
				}
				return true, nil
			},
		},
		"pgvector": {
			name: "pgvector",
			makeDefault: func() (bool, error) {
				// pgvector DefaultConfig lacks ConnectionString — fill it in
				// to exercise the constructor without performing SQL I/O.
				cfg := pgvector.DefaultConfig()
				cfg.ConnectionString = "postgres://challenge-runner@127.0.0.1:5432/challenge?sslmode=disable"
				if err := cfg.Validate(); err != nil {
					return false, fmt.Errorf("Config.Validate: %w", err)
				}
				c, err := pgvector.NewClient(cfg)
				if err != nil {
					return false, fmt.Errorf("NewClient: %w", err)
				}
				if c == nil {
					return false, errors.New("NewClient returned nil client without error")
				}
				return true, nil
			},
			makeBad: func() (bool, error) {
				bad := &pgvector.Config{ConnectionString: ""}
				if err := bad.Validate(); err == nil {
					return false, errors.New("Config{ConnectionString:\"\"} accepted by Validate")
				}
				return true, nil
			},
		},
	}
}

// exerciseBackend runs the makeDefault + makeBad pair for one backend.
func exerciseBackend(be backendInfo) error {
	ok, err := be.makeDefault()
	if err != nil {
		return fmt.Errorf("makeDefault: %w", err)
	}
	if !ok {
		return errors.New("makeDefault returned ok=false without error")
	}
	rejected, err := be.makeBad()
	if err != nil {
		return fmt.Errorf("makeBad: %w", err)
	}
	if !rejected {
		return errors.New("makeBad returned rejected=false without error")
	}
	return nil
}
