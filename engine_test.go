package golem

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// engine_test.go is the primary table-driven suite. It exercises golem's
// supported features and edge cases, asserting the typed-error contract and the
// numeric model.

// --- helpers ---------------------------------------------------------------

// evalAny is a convenience that compiles+runs src on a fresh strict-or-lenient
// engine and returns the raw result and error.
func mustFloat(t *testing.T, e *Engine, src string, vars Vars) float64 {
	t.Helper()
	f, err := mustProgram(t, e, src).EvalFloat(vars)
	if err != nil {
		t.Fatalf("EvalFloat(%q) error: %v", src, err)
	}
	return f
}

func mustProgram(t *testing.T, e *Engine, src string) *Program {
	t.Helper()
	p, err := e.Compile(src)
	if err != nil {
		t.Fatalf("Compile(%q) error: %v", src, err)
	}
	return p
}

// --- the flagship example --------------------------------------------------

func TestSpecFlagshipExample(t *testing.T) {
	// 2 + 3 * (x - 1) with x declared float64; x supplied as int 5 must be
	// coerced to the float64 schema, and the result is 14.
	e := New(WithVariables(Vars{"x": 0.0}))
	p, err := e.Compile("2 + 3 * (x - 1)")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := p.EvalFloat(Vars{"x": 5})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got != 14.0 {
		t.Fatalf("expected 14, got %v", got)
	}
}

// --- arithmetic, precedence, parentheses -----------------------------------

func TestArithmetic(t *testing.T) {
	e := New(WithVariables(Vars{"x": 0.0, "y": 0.0}))
	cases := []struct {
		src  string
		vars Vars
		want float64
	}{
		{"1 + 2 * 3", nil, 7},                  // precedence
		{"(1 + 2) * 3", nil, 9},                // parentheses
		{"2 + 3 * (x - 1)", Vars{"x": 5}, 14},  // flagship example
		{"10 - 4 - 3", nil, 3},                 // left assoc
		{"x / y", Vars{"x": 9, "y": 2}, 4.5},   // float division
		{"x * y + 1", Vars{"x": 2, "y": 3}, 7}, // mixed
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			if got := mustFloat(t, e, tc.src, tc.vars); got != tc.want {
				t.Fatalf("%s = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestModulo(t *testing.T) {
	e := New()
	got, err := e.Eval("17 % 5", nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, _ := got.AsInt(); i != 2 {
		t.Fatalf("17 %% 5 = %v, want 2", i)
	}
}

// --- numeric model: int vs float division (the #1 silent-zero trap) --------

func TestIntegerVsFloatDivision(t *testing.T) {
	e := New()

	// Integer division truncates (documented silent-zero trap): 5/2 == 2, int.
	v, err := e.Eval("5 / 2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, err := v.AsInt(); err != nil || i != 2 {
		t.Fatalf("5/2: got (%v, %v), want (2, nil)", i, err)
	}

	// Float division: 5.0/2 == 2.5.
	v2, err := e.Eval("5.0 / 2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := v2.AsFloat(); err != nil || f != 2.5 {
		t.Fatalf("5.0/2: got (%v, %v), want (2.5, nil)", f, err)
	}
}

// Regression: 2.0 must stay a float. AsInt on an integral float is allowed
// (14.0 -> 14), but the result of a float literal is a float64 and AsFloat must
// return it unchanged; a true float (2.5) must be rejected by AsInt.
func TestRegressionFloatStaysFloat(t *testing.T) {
	e := New()

	v, err := e.Eval("2.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, isFloat := v.AsAny().(float64); !isFloat {
		t.Fatalf("2.0 raw type = %T, want float64", v.AsAny())
	}
	f, err := v.AsFloat()
	if err != nil || f != 2.0 {
		t.Fatalf("AsFloat(2.0) = (%v,%v), want (2,nil)", f, err)
	}

	// A genuinely non-integral float must NOT be silently truncated by AsInt.
	v2, err := e.Eval("2.5", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v2.AsInt(); err == nil {
		t.Fatal("AsInt(2.5) should return TypeMismatchError, got nil")
	} else {
		var tm *TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("AsInt(2.5) error = %T, want *TypeMismatchError", err)
		}
	}

	// An integral float (14.0) MAY be read as int.
	v3, err := e.Eval("7.0 * 2.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, err := v3.AsInt(); err != nil || i != 14 {
		t.Fatalf("AsInt(14.0) = (%v,%v), want (14,nil)", i, err)
	}
}

// --- booleans, ternary, logical operators, short-circuit -------------------

func TestBooleansAndTernary(t *testing.T) {
	e := New(WithVariables(Vars{"status": "", "score": 0.0}))
	cases := []struct {
		src  string
		vars Vars
		want string
	}{
		{`status == "active" and score > 0.8 ? "promote" : "hold"`, Vars{"status": "active", "score": 0.9}, "promote"},
		{`status == "active" and score > 0.8 ? "promote" : "hold"`, Vars{"status": "active", "score": 0.5}, "hold"},
		{`status == "active" and score > 0.8 ? "promote" : "hold"`, Vars{"status": "idle", "score": 0.9}, "hold"},
	}
	for _, tc := range cases {
		t.Run(tc.vars["status"].(string), func(t *testing.T) {
			s, err := mustProgram(t, e, tc.src).EvalString(tc.vars)
			if err != nil {
				t.Fatal(err)
			}
			if s != tc.want {
				t.Fatalf("got %q, want %q", s, tc.want)
			}
		})
	}
}

func TestLogicalOperators(t *testing.T) {
	e := New()
	cases := []struct {
		src  string
		want bool
	}{
		{"true and false", false},
		{"true or false", true},
		{"not true", false},
		{"true && (false || true)", true},
		{"!false", true},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			b, err := mustProgram(t, e, tc.src).EvalBool(nil)
			if err != nil {
				t.Fatal(err)
			}
			if b != tc.want {
				t.Fatalf("%s = %v, want %v", tc.src, b, tc.want)
			}
		})
	}
}

// Boolean short-circuit: a true OR must not evaluate (and thus not panic on)
// the right operand. We use division-by-zero on the right to prove laziness.
func TestBooleanShortCircuit(t *testing.T) {
	e := New(WithVariables(Vars{"n": 0}))

	// true || (1/0 == 0) — if the RHS were evaluated eagerly it would error.
	b, err := e.Eval("true || (1/n == 0)", Vars{"n": 0})
	if err != nil {
		t.Fatalf("short-circuit OR should not evaluate RHS: %v", err)
	}
	if ok, _ := b.AsBool(); !ok {
		t.Fatal("expected true")
	}

	// false && (1/0 == 0) — RHS skipped.
	b2, err := e.Eval("false && (1/n == 0)", Vars{"n": 0})
	if err != nil {
		t.Fatalf("short-circuit AND should not evaluate RHS: %v", err)
	}
	if ok, _ := b2.AsBool(); ok {
		t.Fatal("expected false")
	}
}

// --- strings ---------------------------------------------------------------

func TestStrings(t *testing.T) {
	e := New(WithVariables(Vars{"s": ""}))
	cases := []struct {
		src  string
		vars Vars
		want string
	}{
		{`"foo" + "bar"`, nil, "foobar"},
		{`trim("  hi  ")`, nil, "hi"},
		{`upper("hi")`, nil, "HI"},
		{`lower("HI")`, nil, "hi"},
		{`replace("a-b-c", "-", "_")`, nil, "a_b_c"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			s, err := mustProgram(t, e, tc.src).EvalString(tc.vars)
			if err != nil {
				t.Fatal(err)
			}
			if s != tc.want {
				t.Fatalf("%s = %q, want %q", tc.src, s, tc.want)
			}
		})
	}
}

func TestStringPredicates(t *testing.T) {
	e := New()
	cases := []struct {
		src  string
		want bool
	}{
		{`"hello world" contains "world"`, true}, // contains is an operator
		{`"golem" startsWith "go"`, true},        // startsWith is an operator
		{`"golem" endsWith "em"`, true},          // endsWith is an operator
		{`hasPrefix("golem", "go")`, true},       // hasPrefix is a builtin func
		{`hasSuffix("golem", "em")`, true},       // hasSuffix is a builtin func
		{`"golem" contains "x" or "go" startsWith "g"`, true},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			b, err := mustProgram(t, e, tc.src).EvalBool(nil)
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			if b != tc.want {
				t.Fatalf("%s = %v, want %v", tc.src, b, tc.want)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	e := New()
	v, err := e.Eval(`split("a,b,c", ",")[1]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v.AsString(); s != "b" {
		t.Fatalf(`split(...)[1] = %q, want "b"`, s)
	}
}

func TestUnicodeStrings(t *testing.T) {
	e := New(WithVariables(Vars{"name": ""}))
	v, err := e.Eval(`"héllo, " + name + " 🌍"`, Vars{"name": "wörld"})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v.AsString(); s != "héllo, wörld 🌍" {
		t.Fatalf("unicode concat = %q", s)
	}
	// upper on a unicode string.
	v2, err := e.Eval(`upper("café")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v2.AsString(); s != "CAFÉ" {
		t.Fatalf(`upper("café") = %q, want "CAFÉ"`, s)
	}
}

// --- math: builtins + WithMathStdlib ---------------------------------------

func TestMathBuiltins(t *testing.T) {
	e := New()
	cases := []struct {
		src  string
		want float64
	}{
		{"abs(-5)", 5},
		{"max(3, 7, 2)", 7},
		{"min(3, 7, 2)", 2},
		{"ceil(2.1)", 3},
		{"floor(2.9)", 2},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			if got := mustFloat(t, e, tc.src, nil); got != tc.want {
				t.Fatalf("%s = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestMathStdlib(t *testing.T) {
	e := New(WithMathStdlib())
	cases := []struct {
		src  string
		want float64
	}{
		{"sqrt(16.0)", 4},
		{"pow(2.0, 10.0)", 1024},
		{"log2(8.0)", 3},
		{"log10(1000.0)", 3},
		{"hypot(3.0, 4.0)", 5},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			if got := mustFloat(t, e, tc.src, nil); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("%s = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

// Without WithMathStdlib, sqrt is an unknown name -> the LOUD contract.
func TestMathStdlibNotEnabled(t *testing.T) {
	e := New()
	_, err := e.Compile("sqrt(16.0)")
	if err == nil {
		t.Fatal("expected error: sqrt should be unknown without WithMathStdlib")
	}
}

// --- inline let ------------------------------------------------------------

func TestInlineLet(t *testing.T) {
	e := New(WithVariables(Vars{"price": 0.0}))
	v, err := e.Eval("let discounted = price * 0.9; discounted + 1", Vars{"price": 100})
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := v.AsFloat(); f != 91 {
		t.Fatalf("let result = %v, want 91", f)
	}
}

// --- host-registered functions ---------------------------------------------

func TestCustomFunction(t *testing.T) {
	clamp := func(x, lo, hi float64) float64 {
		if x < lo {
			return lo
		}
		if x > hi {
			return hi
		}
		return x
	}
	e := New(WithMathStdlib(), WithFunction("clamp", clamp))
	got := mustFloat(t, e, "clamp(sqrt(area), 0.0, 100.0)", Vars{"area": 10000.0})
	if got != 100 {
		t.Fatalf("clamp(sqrt(10000)) = %v, want 100", got)
	}
	got2 := mustFloat(t, e, "clamp(sqrt(area), 0.0, 100.0)", Vars{"area": 25.0})
	if got2 != 5 {
		t.Fatalf("clamp(sqrt(25)) = %v, want 5", got2)
	}
}

// A custom function declared with WithVariables and called with an int arg is
// numerically coerced to the float64 the host function declares.
func TestCustomFunctionNumericCoercion(t *testing.T) {
	double := func(x float64) float64 { return x * 2 }
	e := New(WithVariables(Vars{"n": 0.0}), WithFunction("double", double))
	if got := mustFloat(t, e, "double(n)", Vars{"n": 21}); got != 42 {
		t.Fatalf("double(21) = %v, want 42", got)
	}
}

// A custom function returning (T, error) surfaces its error as an EvalError.
func TestCustomFunctionErrorReturn(t *testing.T) {
	boom := func(x float64) (float64, error) {
		if x < 0 {
			return 0, fmt.Errorf("negative input")
		}
		return x, nil
	}
	e := New(WithVariables(Vars{"n": 0.0}), WithFunction("boom", boom))
	_, err := e.Eval("boom(n)", Vars{"n": -1})
	if err == nil {
		t.Fatal("expected EvalError from custom function")
	}
	var ee *EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("error = %T, want *EvalError", err)
	}
}

// A non-func value or variadic func registered with WithFunction is rejected at
// Compile time with a descriptive error (wrapped as EvalError), never panics.
func TestCustomFunctionInvalidRejected(t *testing.T) {
	t.Run("not a func", func(t *testing.T) {
		e := New(WithFunction("nope", 42))
		_, err := e.Compile("1 + 1")
		if err == nil {
			t.Fatal("expected error for non-func WithFunction value")
		}
	})
	t.Run("variadic", func(t *testing.T) {
		e := New(WithFunction("vary", func(xs ...int) int { return len(xs) }))
		_, err := e.Compile("1 + 1")
		if err == nil {
			t.Fatal("expected error for variadic WithFunction value")
		}
	})
}

// --- null / undefined policy -----------------------------------------------

// LOUD by default: a typo'd top-level identifier is a COMPILE error carrying a
// "did you mean" suggestion.
func TestStrictUndefinedVariableIsCompileError(t *testing.T) {
	e := New(WithVariables(Vars{"revenue": 0.0}))
	_, err := e.Compile("revenu * 2")
	if err == nil {
		t.Fatal("expected UndefinedVariableError for typo'd identifier")
	}
	var uv *UndefinedVariableError
	if !errors.As(err, &uv) {
		t.Fatalf("error = %T, want *UndefinedVariableError", err)
	}
	if uv.Name != "revenu" {
		t.Fatalf("Name = %q, want %q", uv.Name, "revenu")
	}
	if uv.Suggestion != "revenue" {
		t.Fatalf("Suggestion = %q, want %q (did-you-mean)", uv.Suggestion, "revenue")
	}
}

// Lenient mode (WithStrictVars(false)): an undeclared identifier resolves to nil
// at runtime rather than failing to compile.
func TestLenientUndefinedVariableIsNil(t *testing.T) {
	e := New(WithStrictVars(false))
	v, err := e.Eval("missing ?? 7", nil)
	if err != nil {
		t.Fatalf("lenient eval error: %v", err)
	}
	// missing is nil, so missing ?? 7 == 7.
	if i, err := v.AsInt(); err != nil || i != 7 {
		t.Fatalf("missing ?? 7 = (%v,%v), want (7,nil)", i, err)
	}
}

// Null-coalescing and optional-chaining: user?.email ?? "no-email".
func TestNullCoalesceAndOptionalChain(t *testing.T) {
	e := New(WithStrictVars(false))

	// user is nil -> user?.email is nil -> ?? falls through to default.
	v, err := e.Eval(`user?.email ?? "no-email"`, Vars{"user": nil})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v.AsString(); s != "no-email" {
		t.Fatalf("got %q, want no-email", s)
	}

	// user present with email -> returns it.
	v2, err := e.Eval(`user?.email ?? "no-email"`, Vars{"user": map[string]any{"email": "a@b.c"}})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v2.AsString(); s != "a@b.c" {
		t.Fatalf("got %q, want a@b.c", s)
	}
}

// Documented caveat: with a map env, an unknown NESTED key silently returns nil
// (only TOP-LEVEL typos are caught). This test pins that documented behavior.
func TestNestedKeyTypoIsSilentNil(t *testing.T) {
	e := New(WithStrictVars(false))
	v, err := e.Eval(`obj.typo ?? "fallback"`, Vars{"obj": map[string]any{"real": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := v.AsString(); s != "fallback" {
		t.Fatalf("nested typo: got %q, want fallback (documented silent-nil)", s)
	}
}

// --- edge cases ------------------------------------------------------------

func TestDivisionByZero(t *testing.T) {
	e := New(WithVariables(Vars{"n": 0}))
	// Integer division by zero panics in Go's runtime; golem's panic boundary
	// must convert it to an EvalError, never let it escape.
	_, err := e.Eval("1 / n", Vars{"n": 0})
	if err == nil {
		t.Fatal("expected EvalError for division by zero")
	}
	var ee *EvalError
	if !errors.As(err, &ee) {
		t.Fatalf("div-by-zero error = %T, want *EvalError", err)
	}
}

func TestNilOperandArithmetic(t *testing.T) {
	e := New(WithStrictVars(false))
	// missing is nil; nil + 1 must produce a typed error, never a panic.
	_, err := e.Eval("missing + 1", nil)
	if err == nil {
		t.Fatal("expected error for nil + 1")
	}
	// Must be one of golem's typed errors.
	if !isTypedGolemError(err) {
		t.Fatalf("nil+1 error = %T, want a golem typed error", err)
	}
}

func TestTypeMismatchStringPlusNumber(t *testing.T) {
	e := New(WithVariables(Vars{"s": "", "n": 0.0}))
	_, err := e.Compile(`s + n`)
	if err == nil {
		t.Fatal("expected TypeMismatchError for string + number")
	}
	var tm *TypeMismatchError
	if !errors.As(err, &tm) {
		t.Fatalf("error = %T, want *TypeMismatchError", err)
	}
}

func TestEmptyAndMissingVars(t *testing.T) {
	e := New()
	// Constant expression with no vars at all.
	v, err := e.Eval("1 + 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if i, _ := v.AsInt(); i != 2 {
		t.Fatalf("1+1 = %v, want 2", i)
	}
	// Empty (non-nil) map.
	v2, err := e.Eval("2 * 3", Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if i, _ := v2.AsInt(); i != 6 {
		t.Fatalf("2*3 = %v, want 6", i)
	}
}

func TestMalformedExpression(t *testing.T) {
	e := New()
	for _, src := range []string{"1 +", "(1 + 2", `"unterminated`, "1 ** "} {
		t.Run(src, func(t *testing.T) {
			_, err := e.Compile(src)
			if err == nil {
				t.Fatalf("expected ParseError for %q", src)
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error = %T, want *ParseError", err)
			}
		})
	}
}

// Input variable supplied with a wrong (non-coercible) type for its declared
// numeric slot -> TypeMismatchError at Eval, not a silent zero.
func TestInputTypeMismatchAgainstSchema(t *testing.T) {
	e := New(WithVariables(Vars{"x": 0.0})) // float64 slot
	_, err := e.Eval("x + 1", Vars{"x": "not a number"})
	if err == nil {
		t.Fatal("expected TypeMismatchError for string in float64 slot")
	}
	var tm *TypeMismatchError
	if !errors.As(err, &tm) {
		t.Fatalf("error = %T, want *TypeMismatchError", err)
	}
}

// --- cost limit ------------------------------------------------------------

func TestCostLimit(t *testing.T) {
	// A tiny node budget makes even a small expression exceed the limit.
	e := New(WithCostLimit(3))
	_, err := e.Compile("1 + 2 + 3 + 4 + 5 + 6 + 7 + 8")
	if err == nil {
		t.Fatal("expected CostLimitError for an expression exceeding the node budget")
	}
	var cl *CostLimitError
	if !errors.As(err, &cl) {
		t.Fatalf("error = %T, want *CostLimitError", err)
	}
}

func TestCostLimitGenerousPasses(t *testing.T) {
	e := New(WithCostLimit(1000))
	if _, err := e.Compile("1 + 2 * 3"); err != nil {
		t.Fatalf("small expression under a generous budget should compile: %v", err)
	}
}

// --- timeout ---------------------------------------------------------------

// Regression: a deadline-exceeding evaluation returns TimeoutError WITHOUT
// the test (caller) goroutine hanging. The abandoned worker goroutine may keep
// running, but the caller returns promptly with TimeoutError.
func TestRegressionTimeoutNoHungCaller(t *testing.T) {
	// A custom function that sleeps far past the deadline.
	slow := func(ms float64) float64 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return ms
	}
	e := New(
		WithVariables(Vars{"ms": 0.0}),
		WithFunction("slow", slow),
		WithEvalTimeout(20*time.Millisecond),
	)

	start := time.Now()
	_, err := e.Eval("slow(ms)", Vars{"ms": 2000})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected TimeoutError")
	}
	var te *TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("error = %T, want *TimeoutError", err)
	}
	// The caller must have returned well before the 2s sleep completed.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("caller hung for %v; timeout did not bound the wait", elapsed)
	}
}

// A fast expression under a timeout completes normally (timeout not triggered).
func TestTimeoutFastExpressionPasses(t *testing.T) {
	e := New(WithEvalTimeout(time.Second))
	v, err := e.Eval("2 + 3 * 4", nil)
	if err != nil {
		t.Fatalf("fast expression under timeout errored: %v", err)
	}
	if i, _ := v.AsInt(); i != 14 {
		t.Fatalf("got %v, want 14", i)
	}
}

// Regression: cost-vs-timeout precedence. When BOTH a cost limit and a
// timeout are set and the cost limit is what trips, the caller sees a
// CostLimitError (not a TimeoutError). The cost guard fires at COMPILE time,
// before any timed run, so it must win.
func TestRegressionCostBeatsTimeoutPrecedence(t *testing.T) {
	e := New(
		WithCostLimit(3),
		WithEvalTimeout(50*time.Millisecond),
	)
	_, err := e.Eval("1 + 2 + 3 + 4 + 5 + 6 + 7 + 8", nil)
	if err == nil {
		t.Fatal("expected an error (cost limit should trip)")
	}
	var cl *CostLimitError
	if !errors.As(err, &cl) {
		t.Fatalf("error = %T, want *CostLimitError (cost must win over timeout)", err)
	}
	var te *TimeoutError
	if errors.As(err, &te) {
		t.Fatal("got TimeoutError; cost limit should have won the precedence")
	}
}

// Regression: a COLLECTION result is reported as an error THROUGH THE
// PYTHON BINDING ENVELOPE. Go-native Eval may return a slice via AsAny, but the
// EvalJSON boundary must classify it as a failure EvalError (collection
// unsupported), never silently emit malformed JSON.
func TestRegressionCollectionResultErrorsViaEnvelope(t *testing.T) {
	e := New()

	// Go-native: a slice result is available via AsAny (no error).
	v, err := e.Eval("[1, 2, 3]", nil)
	if err != nil {
		t.Fatalf("Go-native slice eval errored: %v", err)
	}
	if _, ok := v.AsAny().([]any); !ok {
		t.Fatalf("expected []any result, got %T", v.AsAny())
	}

	// Binding boundary: must be a failure envelope, errtype EvalError.
	out := e.EvalJSON("[1, 2, 3]", "{}")
	if !strings.Contains(out, `"ok":false`) {
		t.Fatalf("collection via EvalJSON should be a failure envelope, got: %s", out)
	}
	if !strings.Contains(out, `"errtype":"EvalError"`) {
		t.Fatalf("collection failure errtype should be EvalError, got: %s", out)
	}
	if !strings.Contains(out, "collection results unsupported") {
		t.Fatalf("expected the documented collection message, got: %s", out)
	}
}

// --- isTypedGolemError -----------------------------------------------------

// isTypedGolemError reports whether err is one of golem's exported typed errors.
func isTypedGolemError(err error) bool {
	var (
		pe *ParseError
		uv *UndefinedVariableError
		tm *TypeMismatchError
		ee *EvalError
		cl *CostLimitError
		te *TimeoutError
	)
	return errors.As(err, &pe) || errors.As(err, &uv) || errors.As(err, &tm) ||
		errors.As(err, &ee) || errors.As(err, &cl) || errors.As(err, &te)
}

// --- typed accessors on Value ----------------------------------------------

func TestValueTypedAccessorsMismatch(t *testing.T) {
	e := New()

	v, _ := e.Eval(`"hello"`, nil)
	if _, err := v.AsBool(); err == nil {
		t.Fatal("AsBool on string should error")
	}
	if _, err := v.AsFloat(); err == nil {
		t.Fatal("AsFloat on string should error")
	}
	if _, err := v.AsInt(); err == nil {
		t.Fatal("AsInt on string should error")
	}

	b, _ := e.Eval("true", nil)
	if _, err := b.AsString(); err == nil {
		t.Fatal("AsString on bool should error")
	}
	// Mismatch errors must be TypeMismatchError, never a panic or nil.
	if _, err := b.AsFloat(); err != nil {
		var tm *TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("AsFloat(bool) = %T, want *TypeMismatchError", err)
		}
	} else {
		t.Fatal("AsFloat(bool) should error")
	}
}

func TestProgramTypedEvalAccessors(t *testing.T) {
	e := New(WithVariables(Vars{"x": 0.0}))
	p := mustProgram(t, e, "x > 10 ? \"big\" : \"small\"")

	s, err := p.EvalString(Vars{"x": 20})
	if err != nil || s != "big" {
		t.Fatalf("EvalString = (%q,%v), want (big,nil)", s, err)
	}

	// EvalBool on a string result -> TypeMismatchError.
	if _, err := p.EvalBool(Vars{"x": 20}); err == nil {
		t.Fatal("EvalBool on a string result should error")
	}
}

func TestValueIsNil(t *testing.T) {
	e := New(WithStrictVars(false))
	v, err := e.Eval("missing", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsNil() {
		t.Fatal("undefined lenient lookup should be nil")
	}
}

// No expr-internal type may leak through any error message.
func TestNoExprTypeLeaksInErrors(t *testing.T) {
	e := New(WithVariables(Vars{"x": 0.0}))
	_, err := e.Compile("nope * 2")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "expr") || strings.Contains(err.Error(), "vm.") {
		t.Fatalf("error message leaks an expr-internal reference: %q", err.Error())
	}
}
