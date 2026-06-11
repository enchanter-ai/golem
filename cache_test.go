package golem

import (
	"fmt"
	"testing"
)

// cache_test.go exercises the bounded LRU program cache: hit/miss behavior,
// per-Engine isolation, and bounded eviction under many distinct expressions
// (no unbounded growth). The cache is a thin use of hashicorp/golang-lru/v2;
// these tests verify golem's wiring of it, not the LRU implementation itself.

// A compiled program is cached: a second Compile of the same source returns a
// Program wrapping the SAME underlying *vm.Program (cache hit).
func TestCompileIsCached(t *testing.T) {
	e := New()
	p1, err := e.Compile("1 + 2")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := e.Compile("1 + 2")
	if err != nil {
		t.Fatal(err)
	}
	if p1.program != p2.program {
		t.Fatal("expected the second Compile to hit the cache and reuse *vm.Program")
	}
}

// Distinct sources produce distinct cached programs.
func TestDistinctSourcesNotShared(t *testing.T) {
	e := New()
	p1, _ := e.Compile("1 + 2")
	p2, _ := e.Compile("3 + 4")
	if p1.program == p2.program {
		t.Fatal("distinct sources must not share a compiled program")
	}
}

// Eval populates the cache so a subsequent Compile of the same source hits it.
func TestEvalPopulatesCache(t *testing.T) {
	e := New()
	if _, err := e.Eval("2 * 21", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.cache.get("2 * 21"); !ok {
		t.Fatal("Eval should have populated the cache for its source")
	}
}

// Caches are per-Engine and never shared across Engines.
func TestCacheIsPerEngine(t *testing.T) {
	e1 := New()
	e2 := New()
	if _, err := e1.Compile("7 + 7"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e2.cache.get("7 + 7"); ok {
		t.Fatal("a second Engine must not see the first Engine's cached program")
	}
}

// The cache is BOUNDED: compiling far more distinct expressions than the
// configured size must not grow the cache past its capacity (LRU eviction).
func TestCacheBoundedEviction(t *testing.T) {
	const size = 8
	e := New(WithCacheSize(size))
	for i := 0; i < size*10; i++ {
		src := fmt.Sprintf("%d + %d", i, i)
		if _, err := e.Compile(src); err != nil {
			t.Fatalf("compile %q: %v", src, err)
		}
	}
	if n := e.cache.lru.Len(); n > size {
		t.Fatalf("cache grew to %d entries, exceeding bound %d", n, size)
	}

	// The earliest entry should have been evicted (LRU); the most recent ones
	// should still be present.
	if _, ok := e.cache.get("0 + 0"); ok {
		t.Fatal("the oldest entry should have been evicted under LRU pressure")
	}
	recent := fmt.Sprintf("%d + %d", size*10-1, size*10-1)
	if _, ok := e.cache.get(recent); !ok {
		t.Fatal("the most recently compiled entry should still be cached")
	}
}

// A non-positive cache size falls back to the default capacity (no panic, no
// zero-capacity cache).
func TestCacheSizeNonPositiveFallsBack(t *testing.T) {
	e := New(WithCacheSize(0))
	if _, err := e.Compile("1 + 1"); err != nil {
		t.Fatalf("zero cache size should fall back to default, got: %v", err)
	}
	if _, ok := e.cache.get("1 + 1"); !ok {
		t.Fatal("default-capacity cache should hold the compiled program")
	}
}

// A cached program still evaluates correctly (the cache hit path must not break
// evaluation).
func TestCachedProgramStillEvaluates(t *testing.T) {
	e := New(WithVariables(Vars{"x": 0.0}))
	if _, err := e.Compile("x + 1"); err != nil {
		t.Fatal(err)
	}
	v, err := e.Eval("x + 1", Vars{"x": 41}) // hits the cache, then runs
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := v.AsFloat(); f != 42 {
		t.Fatalf("cached program eval = %v, want 42", f)
	}
}
