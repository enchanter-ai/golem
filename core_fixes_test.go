package golem

import (
	"errors"
	"math"
	"runtime"
	"testing"
	"time"
)

// These tests cover golem's core numeric and panic-boundary correctness:
//   - int64-typed operands route through golem's truncating integer "/"
//     (5/2 == 2), not expr's native float "/" (2.5).
//   - int64 divide-by-zero is a typed error, not +Inf.
//   - math.MinInt64 / -1 is a typed OverflowError, not a silent wrap.
//   - a custom function that calls runtime.Goexit() makes Eval return a typed
//     EvalError instead of hanging.

// cf1Engine builds an engine with an int64-typed declared schema so expr's
// checker resolves "a / b" to golem's int64 division witness (and not expr's
// native float "/"). coerceToDeclared preserves int64 inputs unchanged, so the
// operands reach golemDiv as int64.
func cf1Engine(t *testing.T, opts ...Option) *Engine {
	t.Helper()
	schema := Vars{"a": int64(0), "b": int64(0)}
	return New(append([]Option{WithVariables(schema)}, opts...)...)
}

func TestCoreFix_Int64TruncatingDivision(t *testing.T) {
	e := cf1Engine(t)
	// Both operands int64; expect truncating integer division: 5/2 == 2.
	v, err := e.Eval("a / b", Vars{"a": int64(5), "b": int64(2)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := v.AsInt()
	if err != nil {
		t.Fatalf("int64 5/2 result not integral (likely float division): %v", err)
	}
	if got != 2 {
		t.Fatalf("int64 5/2: got %d, want 2 (truncating integer division, not 2.5)", got)
	}

	// Sanity: a float-typed divisor still uses float division (5 / 2.0 == 2.5).
	ef := New(WithVariables(Vars{"a": int64(0), "b": float64(0)}))
	fv, err := ef.Eval("a / b", Vars{"a": int64(5), "b": float64(2)})
	if err != nil {
		t.Fatalf("unexpected error on mixed operands: %v", err)
	}
	f, err := fv.AsFloat()
	if err != nil {
		t.Fatalf("mixed operand result: %v", err)
	}
	if f != 2.5 {
		t.Fatalf("int64/float 5/2.0: got %v, want 2.5", f)
	}
}

func TestCoreFix_Int64DivideByZeroTypedError(t *testing.T) {
	e := cf1Engine(t)
	v, err := e.Eval("a / b", Vars{"a": int64(10), "b": int64(0)})
	if err == nil {
		t.Fatalf("int64 10/0: expected a typed error, got value %#v (must not be +Inf)", v)
	}
	// Must NOT silently produce +Inf.
	if f, ferr := v.AsFloat(); ferr == nil && math.IsInf(f, 0) {
		t.Fatalf("int64 10/0: produced %v (+Inf) instead of a typed error", f)
	}
	var evalErr *EvalError
	if !errors.As(err, &evalErr) {
		t.Fatalf("int64 10/0: expected *EvalError, got %T: %v", err, err)
	}
}

func TestCoreFix_MinInt64DivNegOneOverflow(t *testing.T) {
	e := cf1Engine(t)
	_, err := e.Eval("a / b", Vars{"a": int64(math.MinInt64), "b": int64(-1)})
	if err == nil {
		t.Fatalf("MinInt64 / -1: expected an OverflowError, got nil (silent wrap)")
	}
	var ovf *OverflowError
	if !errors.As(err, &ovf) {
		t.Fatalf("MinInt64 / -1: expected *OverflowError, got %T: %v", err, err)
	}
	if ovf.Op != "division" {
		t.Fatalf("OverflowError.Op = %q, want %q", ovf.Op, "division")
	}
}

func TestCoreFix_GoexitInCustomFnReturnsEvalError(t *testing.T) {
	// A custom function that calls runtime.Goexit() must NOT hang Eval: the
	// worker goroutine exits without delivering a completed result and the
	// run path surfaces a typed EvalError.
	exiter := func() int {
		runtime.Goexit()
		return 0 // unreachable
	}
	e := New(WithStrictVars(false), WithFunction("cf_goexit", exiter))

	done := make(chan struct{})
	var (
		gotErr error
		gotVal Value
	)
	go func() {
		defer close(done)
		gotVal, gotErr = e.Eval("cf_goexit()", nil)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Eval hung on a custom function that called runtime.Goexit()")
	}

	if gotErr == nil {
		t.Fatalf("Goexit custom fn: expected an EvalError, got value %#v", gotVal)
	}
	var evalErr *EvalError
	if !errors.As(gotErr, &evalErr) {
		t.Fatalf("Goexit custom fn: expected *EvalError, got %T: %v", gotErr, gotErr)
	}
}
