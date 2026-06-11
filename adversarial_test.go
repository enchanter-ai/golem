package golem

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// adversarial_test.go is a break-it suite: each case feeds golem an input chosen
// to expose a silent-wrong-result, a panic escape, a hang, or a type confusion,
// and asserts the SAFE outcome (a typed error, a bounded result, or a documented
// value). A failure here is a real defect, not a style nit.

func TestAdversarial(t *testing.T) {
	t.Run("int divide-by-zero is a typed error, not +Inf", func(t *testing.T) {
		e := New(WithVariables(Vars{"a": 0, "b": 0}))
		if _, err := e.Eval("a / b", Vars{"a": 10, "b": 0}); err == nil {
			t.Fatal("int 10/0 returned no error (silent +Inf); want a typed error")
		}
	})

	t.Run("int64 divide-by-zero is a typed error", func(t *testing.T) {
		e := New(WithVariables(Vars{"a": int64(0), "b": int64(0)}))
		if _, err := e.Eval("a / b", Vars{"a": int64(10), "b": int64(0)}); err == nil {
			t.Fatal("int64 10/0 returned no error; want a typed error")
		}
	})

	t.Run("MinInt64 / -1 is an OverflowError, not a silent wrap", func(t *testing.T) {
		e := New(WithVariables(Vars{"a": int64(0), "b": int64(0)}))
		_, err := e.Eval("a / b", Vars{"a": int64(math.MinInt64), "b": int64(-1)})
		if err == nil {
			t.Fatal("MinInt64/-1 returned no error; want OverflowError")
		}
	})

	t.Run("uint64 result above MaxInt64 must not silently wrap negative", func(t *testing.T) {
		e := New(WithFunction("bignum", func() uint64 { return math.MaxUint64 }))
		v, err := e.Eval("bignum()", nil)
		if err != nil {
			t.Fatalf("eval failed: %v", err)
		}
		n, ierr := v.AsInt()
		if ierr == nil {
			t.Fatalf("AsInt() on uint64(MaxUint64) returned (%d, nil); a value above MaxInt64 must yield an overflow error, not a silently wrapped %d", n, n)
		}
	})

	t.Run("undefined variable (strict) is a compile-time error", func(t *testing.T) {
		e := New(WithVariables(Vars{"x": 0.0}))
		if _, err := e.Compile("ghost * 2"); err == nil {
			t.Fatal("undeclared 'ghost' compiled cleanly; want UndefinedVariableError")
		}
	})

	t.Run("undefined variable (lenient) coalesces to a value", func(t *testing.T) {
		e := New(WithStrictVars(false))
		v, err := e.Eval(`ghost ?? 7`, nil)
		if err != nil {
			t.Fatalf("lenient eval failed: %v", err)
		}
		if f, _ := v.AsFloat(); f != 7 {
			t.Fatalf("ghost ?? 7 = %v; want 7", f)
		}
	})

	t.Run("string + number is a type error, not a coercion", func(t *testing.T) {
		e := New(WithVariables(Vars{"n": 0}))
		if _, err := e.Compile(`"x" + n`); err == nil {
			t.Fatal(`"x" + n compiled cleanly; want a TypeMismatchError`)
		}
	})

	t.Run("pathological node count is bounded by the cost limit", func(t *testing.T) {
		e := New(WithVariables(Vars{"x": 0.0}), WithCostLimit(20))
		big := "x" + strings.Repeat(" + x", 200)
		if _, err := e.Compile(big); err == nil {
			t.Fatal("a 200-term expression compiled under WithCostLimit(20); want CostLimitError")
		}
	})

	t.Run("a ctx-ignoring slow function is bounded by the timeout, no hang", func(t *testing.T) {
		e := New(WithEvalTimeout(20*time.Millisecond),
			WithFunction("slow", func() int { time.Sleep(2 * time.Second); return 1 }))
		done := make(chan error, 1)
		go func() { _, err := e.Eval("slow()", nil); done <- err }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("slow() under a 20ms timeout returned no error; want TimeoutError")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Eval hung past the timeout (caller not released)")
		}
	})

	t.Run("malformed expression is a ParseError", func(t *testing.T) {
		e := New()
		if _, err := e.Compile("2 +"); err == nil {
			t.Fatal("'2 +' compiled cleanly; want ParseError")
		}
	})

	t.Run("empty expression is rejected", func(t *testing.T) {
		e := New()
		if _, err := e.Compile(""); err == nil {
			t.Fatal("empty source compiled cleanly; want an error")
		}
	})

	t.Run("a panicking custom function becomes a typed error, not a crash", func(t *testing.T) {
		e := New(WithFunction("boom", func() int { panic("kaboom") }))
		if _, err := e.Eval("boom()", nil); err == nil {
			t.Fatal("a panicking custom fn returned no error; want EvalError from the panic boundary")
		}
	})

	t.Run("EvalJSON rejects a collection result via the envelope", func(t *testing.T) {
		e := New()
		env := e.EvalJSON("[1, 2, 3]", "{}")
		if !strings.Contains(env, `"ok":false`) {
			t.Fatalf("EvalJSON of a list should be a failure envelope; got %s", env)
		}
	})

	t.Run("EvalJSON rejects a NaN/Inf result via the envelope", func(t *testing.T) {
		e := New(WithMathStdlib())
		env := e.EvalJSON("sqrt(-1.0)", "{}")
		if !strings.Contains(env, `"ok":false`) {
			t.Fatalf("EvalJSON of NaN should be a failure envelope; got %s", env)
		}
	})

	t.Run("malformed varsJSON is a failure envelope, not a panic", func(t *testing.T) {
		e := New(WithStrictVars(false))
		env := e.EvalJSON("x", "{not json")
		if !strings.Contains(env, `"ok":false`) {
			t.Fatalf("malformed varsJSON should be a failure envelope; got %s", env)
		}
	})

	t.Run("one compiled program is safe under concurrent evaluation", func(t *testing.T) {
		e := New(WithVariables(Vars{"x": 0.0}))
		p, err := e.Compile("2 + 3 * (x - 1)")
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 64)
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				if _, err := p.Eval(Vars{"x": float64(n)}); err != nil {
					errs <- err
				}
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent Eval errored: %v", err)
		}
	})
}
