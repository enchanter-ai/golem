package golem

import (
	"testing"
	"time"
)

// fuzz_test.go fuzzes the compile + eval path. The hard contract under fuzzing:
// NO panic ever escapes golem — every malformed, hostile, or pathological input
// must yield either a clean result or one of golem's typed errors. Run with:
//   go test -fuzz=FuzzCompile -fuzztime=30s

// FuzzCompile feeds arbitrary strings to Compile and (when compilation
// succeeds) to Eval, asserting only that golem never panics and never lets an
// expr-internal panic escape its boundary.
func FuzzCompile(f *testing.F) {
	// Seed corpus: valid, invalid, hostile, and edge inputs.
	seeds := []string{
		"",
		"2 + 3 * (x - 1)",
		"1 / 0",
		"1 +",
		"(((((1)))))",
		`"unterminated`,
		`user?.email ?? "no-email"`,
		"let a = 1; a + a",
		"true and false or not true",
		`split("a,b,c", ",")`,
		"sqrt(-1.0)",            // NaN-producing
		"1e308 * 1e308",         // Inf-producing
		"5 / 2",                 // int division trap
		"2.0",                   // float literal
		"x ?? y ?? z",           // chained coalesce
		"💥 + 🔥",                 // unicode garbage
		"\x00\x01\x02",          // control bytes
		"a" + repeat("+a", 200), // deep arithmetic chain
	}
	for _, s := range seeds {
		f.Add(s)
	}

	// Strict and lenient engines, the latter with a math stdlib, a cost limit,
	// and a short timeout so the cost/timeout guards are exercised under fuzz.
	strict := New(WithVariables(Vars{"x": 0.0, "y": 0.0, "z": 0.0}))
	lenient := New(
		WithStrictVars(false),
		WithMathStdlib(),
		WithCostLimit(10_000),
		WithEvalTimeout(250*time.Millisecond),
	)

	f.Fuzz(func(t *testing.T, src string) {
		// 1. Compile must never panic; any failure is a typed golem error.
		p, err := strict.Compile(src)
		if err != nil && !isTypedGolemError(err) {
			t.Fatalf("Compile(%q) returned a non-golem error type: %T (%v)", src, err, err)
		}
		if err == nil {
			// Eval must never panic, even on hostile input.
			if _, evErr := p.Eval(Vars{"x": 1.0, "y": 2.0, "z": 3.0}); evErr != nil && !isTypedGolemError(evErr) {
				t.Fatalf("Eval(%q) returned a non-golem error type: %T (%v)", src, evErr, evErr)
			}
		}

		// 2. The lenient engine path: compile+eval, with limits engaged.
		lp, lerr := lenient.Compile(src)
		if lerr != nil && !isTypedGolemError(lerr) {
			t.Fatalf("lenient Compile(%q) non-golem error: %T (%v)", src, lerr, lerr)
		}
		if lerr == nil {
			if _, evErr := lp.Eval(Vars{"x": 1.0}); evErr != nil && !isTypedGolemError(evErr) {
				t.Fatalf("lenient Eval(%q) non-golem error: %T (%v)", src, evErr, evErr)
			}
		}

		// 3. The binding boundary must NEVER panic and must always return a
		// parseable, non-empty JSON envelope string.
		out := lenient.EvalJSON(src, `{"x":1,"y":2,"z":3}`)
		if out == "" {
			t.Fatalf("EvalJSON(%q) returned an empty envelope", src)
		}
	})
}

// repeat returns s concatenated n times (a tiny helper to build a deep chain in
// the seed corpus without importing strings into the fuzz seed list).
func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
