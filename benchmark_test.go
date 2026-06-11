package golem

import (
	"fmt"
	"testing"
)

// benchmark_test.go reports ns/op and allocs/op for the three axes that define
// golem's compile-once / run-many performance profile: cold Compile, cached
// Eval (the hot path), and concurrent cached Eval. Run with:
//   go test -bench=. -benchmem

const benchExpr = "2 + 3 * (x - 1)"

// BenchmarkCompileCold measures a full parse + type-check on every iteration
// (cache defeated by a unique source each time). This is the cost that the
// program cache amortizes away on the hot path.
func BenchmarkCompileCold(b *testing.B) {
	e := New(WithVariables(Vars{"x": 0.0}))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// A unique source per iteration forces a real compile (cache miss).
		src := fmt.Sprintf("2 + 3 * (x - %d)", i)
		if _, err := e.Compile(src); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEvalCached measures the hot path: a single precompiled Program
// evaluated repeatedly. This is the millions/sec, tens-of-ns claim.
func BenchmarkEvalCached(b *testing.B) {
	e := New(WithVariables(Vars{"x": 0.0}))
	p, err := e.Compile(benchExpr)
	if err != nil {
		b.Fatal(err)
	}
	vars := Vars{"x": 5.0}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.EvalFloat(vars); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEvalEngineCached measures Engine.Eval (compile-with-cache-hit + run),
// i.e. the cost a caller pays going through the Engine rather than holding a
// Program, dominated by the cache lookup plus the run.
func BenchmarkEvalEngineCached(b *testing.B) {
	e := New(WithVariables(Vars{"x": 0.0}))
	if _, err := e.Compile(benchExpr); err != nil { // warm the cache
		b.Fatal(err)
	}
	vars := Vars{"x": 5.0}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Eval(benchExpr, vars); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEvalConcurrent measures the cached hot path under parallel load —
// many goroutines sharing ONE Program — to show the per-eval cost stays flat
// (no shared mutable state, no lock contention beyond the env-map allocation).
func BenchmarkEvalConcurrent(b *testing.B) {
	e := New(WithVariables(Vars{"x": 0.0}))
	p, err := e.Compile(benchExpr)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		vars := Vars{"x": 5.0}
		for pb.Next() {
			if _, err := p.EvalFloat(vars); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCompileVsCached prints the cold-vs-cached ratio directly so the
// "big speedup vs cold" claim is reproducible from one run. It is reported as a
// sub-benchmark pair; the ratio is read off the two ns/op lines.
func BenchmarkCompileVsCached(b *testing.B) {
	e := New(WithVariables(Vars{"x": 0.0}))
	p, _ := e.Compile(benchExpr)
	vars := Vars{"x": 5.0}

	b.Run("cold-compile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			src := fmt.Sprintf("2 + 3 * (x - %d)", i)
			if _, err := e.Compile(src); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cached-eval", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := p.EvalFloat(vars); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkEvalCustomFunction measures the hot path through a host-registered
// function (the reflect-adapted call) so the function-call overhead is visible.
func BenchmarkEvalCustomFunction(b *testing.B) {
	clamp := func(x, lo, hi float64) float64 {
		if x < lo {
			return lo
		}
		if x > hi {
			return hi
		}
		return x
	}
	e := New(WithVariables(Vars{"x": 0.0}), WithFunction("clamp", clamp))
	p, err := e.Compile("clamp(x, 0.0, 10.0)")
	if err != nil {
		b.Fatal(err)
	}
	vars := Vars{"x": 5.0}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.EvalFloat(vars); err != nil {
			b.Fatal(err)
		}
	}
}
