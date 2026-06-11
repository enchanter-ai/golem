package golem

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// concurrency_test.go is designed to pass `go test -race`. Its central claim:
// a single compiled *Program is safe to Eval from many goroutines at once
// (expr's *vm.Program is reusable and thread-safe; golem adds no per-Eval shared
// mutable state). Run with: go test -race -run Concurren .

const (
	goroutines     = 64
	evalsPerWorker = 200
)

// One shared Program evaluated concurrently from many goroutines must produce
// the correct, deterministic result every time, with no data race.
func TestConcurrentEvalSharedProgram(t *testing.T) {
	e := New(WithVariables(Vars{"x": 0.0}))
	p, err := e.Compile("2 + 3 * (x - 1)") // the flagship expression
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var failures int64
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < evalsPerWorker; i++ {
				x := float64((seed + i) % 100)
				want := 2 + 3*(x-1)
				// Each goroutine supplies its OWN vars map; only the Program is
				// shared. buildEnv copies vars into a fresh map per call.
				got, err := p.EvalFloat(Vars{"x": x})
				if err != nil || got != want {
					atomic.AddInt64(&failures, 1)
				}
			}
		}(g)
	}
	wg.Wait()

	if failures != 0 {
		t.Fatalf("%d concurrent evaluations returned a wrong result or error", failures)
	}
}

// Concurrent Eval through the Engine (compile+cache+run) from many goroutines,
// mixing cache hits and a few distinct sources, must be race-free.
func TestConcurrentEngineEval(t *testing.T) {
	e := New(
		WithVariables(Vars{"a": 0.0, "b": 0.0}),
		WithCacheSize(4), // small, to force concurrent eviction under contention
	)
	sources := []string{
		"a + b",
		"a * b",
		"a - b",
		"a / (b + 1)",
		"a + b * 2",
		"(a - b) * 3",
	}

	var wg sync.WaitGroup
	var failures int64
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < evalsPerWorker; i++ {
				src := sources[(seed+i)%len(sources)]
				_, err := e.Eval(src, Vars{"a": float64(seed), "b": float64(i)})
				if err != nil {
					atomic.AddInt64(&failures, 1)
				}
			}
		}(g)
	}
	wg.Wait()

	if failures != 0 {
		t.Fatalf("%d concurrent engine evaluations errored", failures)
	}
}

// Concurrent Eval of a Program that uses a custom function and the math stdlib —
// the function adapter and option set are read-only after New, so this must be
// race-free too.
func TestConcurrentCustomFunction(t *testing.T) {
	clamp := func(x, lo, hi float64) float64 {
		switch {
		case x < lo:
			return lo
		case x > hi:
			return hi
		default:
			return x
		}
	}
	e := New(
		WithMathStdlib(),
		WithVariables(Vars{"area": 0.0}),
		WithFunction("clamp", clamp),
	)
	p, err := e.Compile("clamp(sqrt(area), 0.0, 100.0)")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var failures int64
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < evalsPerWorker; i++ {
				if _, err := p.EvalFloat(Vars{"area": float64((seed + i) * 4)}); err != nil {
					atomic.AddInt64(&failures, 1)
				}
			}
		}(g)
	}
	wg.Wait()

	if failures != 0 {
		t.Fatalf("%d concurrent custom-function evaluations errored", failures)
	}
}

// Concurrent Eval under a timeout: the goroutine-race timeout path must not race
// on the shared Program, and fast expressions must all succeed within the
// deadline. (The abandoned-goroutine path is covered by the regression test in
// engine_test.go.)
func TestConcurrentEvalWithTimeout(t *testing.T) {
	e := New(
		WithVariables(Vars{"x": 0.0}),
		WithEvalTimeout(2*time.Second),
	)
	p, err := e.Compile("x * x + 1")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var failures int64
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < evalsPerWorker; i++ {
				if _, err := p.EvalFloat(Vars{"x": float64(seed + i)}); err != nil {
					atomic.AddInt64(&failures, 1)
				}
			}
		}(g)
	}
	wg.Wait()

	if failures != 0 {
		t.Fatalf("%d concurrent timed evaluations errored", failures)
	}
}
