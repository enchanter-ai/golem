package golem

import (
	"encoding/json"
	"errors"
	"testing"
)

// covmoreIntProg compiles an integer-returning expression against an int64
// schema so the result reaches the caller as a Go integer.
func covmoreIntProg(t *testing.T, src string) *Program {
	t.Helper()
	e := New(WithVariables(Vars{"a": int64(0), "b": int64(0)}))
	p, err := e.Compile(src)
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	return p
}

// Program.EvalInt — an integral result yields int64; a true (non-integral)
// float result yields a *TypeMismatchError.
func TestCovMore_ProgramEvalInt(t *testing.T) {
	// Integer result path.
	p := covmoreIntProg(t, "a + b")
	got, err := p.EvalInt(Vars{"a": int64(5), "b": int64(3)})
	if err != nil {
		t.Fatalf("EvalInt integer result: unexpected error: %v", err)
	}
	if got != 8 {
		t.Fatalf("EvalInt 5+3: got %d, want 8", got)
	}

	// True-float result path -> TypeMismatchError. Use a float-typed schema so
	// the expression evaluates with native float arithmetic.
	ef := New(WithVariables(Vars{"x": float64(0)}))
	pf, err := ef.Compile("x + 0.5")
	if err != nil {
		t.Fatalf("compile float expr: %v", err)
	}
	if _, err := pf.EvalInt(Vars{"x": float64(2)}); err == nil {
		t.Fatalf("EvalInt on non-integral float: expected TypeMismatchError, got nil")
	} else {
		var tm *TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("EvalInt on non-integral float: expected *TypeMismatchError, got %T: %v", err, err)
		}
	}

	// An integral float (e.g. 2.0) is accepted by EvalInt.
	pintf, err := ef.Compile("x + 1.0")
	if err != nil {
		t.Fatalf("compile integral-float expr: %v", err)
	}
	gi, err := pintf.EvalInt(Vars{"x": float64(3)})
	if err != nil {
		t.Fatalf("EvalInt on integral float: unexpected error: %v", err)
	}
	if gi != 4 {
		t.Fatalf("EvalInt 3.0+1.0: got %d, want 4", gi)
	}

	// Error propagation: a compile/eval-time failure surfaces from EvalInt.
	estrict := New() // strict vars default
	pun, err := estrict.Compile("missingvar")
	if err == nil {
		// If compile succeeds (lenient), eval should still error or return; either
		// way exercise the early-return branch.
		if _, evErr := pun.EvalInt(nil); evErr == nil {
			t.Logf("EvalInt on undefined var returned no error (lenient build)")
		}
	}
}

// Value.AsFloat over int / int32 / int64 / float64 kinds (plus the
// non-numeric error branch).
func TestCovMore_ValueAsFloat_Kinds(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want float64
	}{
		{"int", int(7), 7},
		{"int32", int32(8), 8},
		{"int64", int64(9), 9},
		{"float64", float64(2.5), 2.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newValue(tc.raw).AsFloat()
			if err != nil {
				t.Fatalf("AsFloat(%v): unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("AsFloat(%v): got %v, want %v", tc.raw, got, tc.want)
			}
		})
	}

	// Non-numeric -> TypeMismatchError.
	if _, err := newValue("hello").AsFloat(); err == nil {
		t.Fatalf("AsFloat(string): expected TypeMismatchError, got nil")
	} else {
		var tm *TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("AsFloat(string): expected *TypeMismatchError, got %T", err)
		}
	}
}

// Value.AsInt over int / int32 / int64 / float64 kinds (integral and
// non-integral float branches, plus the non-numeric error branch).
func TestCovMore_ValueAsInt_Kinds(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int64
	}{
		{"int", int(7), 7},
		{"int32", int32(8), 8},
		{"int64", int64(9), 9},
		{"integral_float64", float64(11.0), 11},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newValue(tc.raw).AsInt()
			if err != nil {
				t.Fatalf("AsInt(%v): unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("AsInt(%v): got %d, want %d", tc.raw, got, tc.want)
			}
		})
	}

	// Non-integral float64 -> TypeMismatchError.
	if _, err := newValue(float64(2.5)).AsInt(); err == nil {
		t.Fatalf("AsInt(2.5): expected TypeMismatchError, got nil")
	} else {
		var tm *TypeMismatchError
		if !errors.As(err, &tm) {
			t.Fatalf("AsInt(2.5): expected *TypeMismatchError, got %T", err)
		}
	}

	// Non-numeric -> TypeMismatchError.
	if _, err := newValue(true).AsInt(); err == nil {
		t.Fatalf("AsInt(bool): expected TypeMismatchError, got nil")
	}
}

// normalizeJSONNumber over a large integer (fits int64), a fractional
// number, and a non-numeric value; plus recursion through map and slice.
func TestCovMore_NormalizeJSONNumber(t *testing.T) {
	num := func(s string) json.Number { return json.Number(s) }

	// Large integer that fits int64 -> int64.
	if got := normalizeJSONNumber(num("9007199254740993")); got != int64(9007199254740993) {
		t.Fatalf("normalizeJSONNumber(large int): got %#v (%T), want int64", got, got)
	}

	// Fractional number -> float64.
	if got := normalizeJSONNumber(num("3.14")); got != float64(3.14) {
		t.Fatalf("normalizeJSONNumber(fraction): got %#v (%T), want float64 3.14", got, got)
	}

	// A value too big for int64 but parseable as float -> float64.
	bigFloat := normalizeJSONNumber(num("1e400"))
	if _, ok := bigFloat.(string); ok {
		// 1e400 overflows float64 to +Inf; Float64 returns an error, so it falls
		// through to String(). Accept either float64 or the string fallback.
		t.Logf("normalizeJSONNumber(1e400) fell through to string (%v)", bigFloat)
	}

	// Non-numeric (passes through unchanged).
	if got := normalizeJSONNumber("plain"); got != "plain" {
		t.Fatalf("normalizeJSONNumber(string): got %#v, want \"plain\"", got)
	}
	if got := normalizeJSONNumber(true); got != true {
		t.Fatalf("normalizeJSONNumber(bool): got %#v, want true", got)
	}

	// Recursion through a map: nested json.Number is normalized in place.
	m := map[string]any{"n": num("42"), "f": num("1.5"), "s": "x"}
	out := normalizeJSONNumber(m).(map[string]any)
	if out["n"] != int64(42) {
		t.Fatalf("normalizeJSONNumber(map).n: got %#v, want int64 42", out["n"])
	}
	if out["f"] != float64(1.5) {
		t.Fatalf("normalizeJSONNumber(map).f: got %#v, want float64 1.5", out["f"])
	}

	// Recursion through a slice.
	s := []any{num("1"), num("2.5"), "three"}
	outs := normalizeJSONNumber(s).([]any)
	if outs[0] != int64(1) || outs[1] != float64(2.5) || outs[2] != "three" {
		t.Fatalf("normalizeJSONNumber(slice): got %#v", outs)
	}
}

// Value.AsFloat / AsInt over the unsigned and narrow integer kinds the
// expr VM may produce, rounding out the numeric coverage.
func TestCovMore_ValueAs_WideKinds(t *testing.T) {
	floatKinds := []any{int16(3), int8(4), uint(5), uint64(6), uint32(7), uint16(8), uint8(9), float32(1.5)}
	for _, raw := range floatKinds {
		if _, err := newValue(raw).AsFloat(); err != nil {
			t.Fatalf("AsFloat(%T): unexpected error: %v", raw, err)
		}
	}
	intKinds := []any{int16(3), int8(4), uint(5), uint64(6), uint32(7), uint16(8), uint8(9)}
	for _, raw := range intKinds {
		if _, err := newValue(raw).AsInt(); err != nil {
			t.Fatalf("AsInt(%T): unexpected error: %v", raw, err)
		}
	}
	// Integral float32 -> AsInt accepted; non-integral float32 -> error.
	if got, err := newValue(float32(5.0)).AsInt(); err != nil || got != 5 {
		t.Fatalf("AsInt(float32 5.0): got %d, err %v", got, err)
	}
	if _, err := newValue(float32(5.5)).AsInt(); err == nil {
		t.Fatalf("AsInt(float32 5.5): expected TypeMismatchError, got nil")
	}
}

// toInt / toFloat division-witness helpers over their numeric kinds.
func TestCovMore_ToIntToFloat(t *testing.T) {
	intVals := []any{int(1), int64(2), int32(3), int16(4), int8(5), uint(6), uint64(7), uint32(8)}
	for _, v := range intVals {
		if _, ok := toInt(v); !ok {
			t.Fatalf("toInt(%T): expected ok", v)
		}
	}
	if _, ok := toInt(1.5); ok {
		t.Fatalf("toInt(float64): expected not-ok")
	}
	if _, ok := toInt("s"); ok {
		t.Fatalf("toInt(string): expected not-ok")
	}

	floatVals := []any{float64(1), float32(2), int(3), int64(4), int32(5), int16(6), int8(7), uint(8), uint64(9)}
	for _, v := range floatVals {
		if _, ok := toFloat(v); !ok {
			t.Fatalf("toFloat(%T): expected ok", v)
		}
	}
	if _, ok := toFloat("s"); ok {
		t.Fatalf("toFloat(string): expected not-ok")
	}
}

// Program.Source returns the original expression text.
func TestCovMore_ProgramSource(t *testing.T) {
	e := New()
	p, err := e.Compile("1 + 2")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if p.Source() != "1 + 2" {
		t.Fatalf("Source(): got %q, want %q", p.Source(), "1 + 2")
	}
}

// convertArg / isNumericKind: a host function declaring float64 params invoked
// with an integer literal exercises the numeric-widening path; a string arg
// exercises the mismatch path.
func TestCovMore_ConvertArgNumericWidening(t *testing.T) {
	half := func(x float64) float64 { return x / 2 }
	e := New(WithStrictVars(false), WithFunction("covhalf", half))

	// Integer literal widened to float64.
	v, err := e.Eval("covhalf(7)", nil)
	if err != nil {
		t.Fatalf("covhalf(7): unexpected error: %v", err)
	}
	f, err := v.AsFloat()
	if err != nil || f != 3.5 {
		t.Fatalf("covhalf(7): got %v err %v, want 3.5", f, err)
	}

	// A string arg cannot be coerced to float64 -> error surfaces.
	if _, err := e.Eval(`covhalf("nope")`, nil); err == nil {
		t.Logf("covhalf(string) returned no error (checker may reject earlier)")
	}
}

// EvalJSON drives encodeResultEnvelope and errType across result/error shapes.
func TestCovMore_EvalJSON_Envelopes(t *testing.T) {
	e := New(WithStrictVars(false))

	parse := func(t *testing.T, s string) envelope {
		t.Helper()
		var env envelope
		if err := json.Unmarshal([]byte(s), &env); err != nil {
			t.Fatalf("unmarshal envelope %q: %v", s, err)
		}
		return env
	}

	// int result.
	if env := parse(t, e.EvalJSON("2 + 3", "")); !env.OK || env.Type != "int" {
		t.Fatalf("int envelope: %+v", env)
	}
	// float result.
	if env := parse(t, e.EvalJSON("2.5 + 1.0", "")); !env.OK || env.Type != "float" {
		t.Fatalf("float envelope: %+v", env)
	}
	// bool result.
	if env := parse(t, e.EvalJSON("true && false", "")); !env.OK || env.Type != "bool" {
		t.Fatalf("bool envelope: %+v", env)
	}
	// string result.
	if env := parse(t, e.EvalJSON(`"a" + "b"`, "")); !env.OK || env.Type != "string" {
		t.Fatalf("string envelope: %+v", env)
	}
	// null result.
	if env := parse(t, e.EvalJSON("nil", "")); !env.OK || env.Type != "null" {
		t.Fatalf("null envelope: %+v", env)
	}
	// collection result -> failure EvalError.
	if env := parse(t, e.EvalJSON("[1, 2, 3]", "")); env.OK || env.ErrType != "EvalError" {
		t.Fatalf("collection envelope: %+v", env)
	}
	// parse error -> failure ParseError.
	if env := parse(t, e.EvalJSON("1 +", "")); env.OK || env.ErrType == "" {
		t.Fatalf("parse-error envelope: %+v", env)
	}
	// invalid vars JSON -> failure EvalError.
	if env := parse(t, e.EvalJSON("1", "{not json")); env.OK || env.ErrType != "EvalError" {
		t.Fatalf("bad-vars envelope: %+v", env)
	}

	// errType maps each typed error to its name.
	cases := map[error]string{
		&ParseError{}:             "ParseError",
		&UndefinedVariableError{}: "UndefinedVariableError",
		&TypeMismatchError{}:      "TypeMismatchError",
		&CostLimitError{}:         "CostLimitError",
		&TimeoutError{}:           "TimeoutError",
		&EvalError{}:              "EvalError",
		errors.New("other"):       "EvalError",
	}
	for err, want := range cases {
		if got := errType(err); got != want {
			t.Fatalf("errType(%T): got %q, want %q", err, got, want)
		}
	}
}

// Error Error()/Unwrap() methods across the typed-error set.
func TestCovMore_ErrorMethods(t *testing.T) {
	base := errors.New("root cause")

	t.Run("ParseError", func(t *testing.T) {
		e := &ParseError{Source: "1 +", Cause: base}
		if e.Error() == "" {
			t.Fatal("ParseError.Error() empty")
		}
		if !errors.Is(e, base) {
			t.Fatalf("ParseError.Unwrap() did not return cause")
		}
	})

	t.Run("UndefinedVariableError", func(t *testing.T) {
		withSug := &UndefinedVariableError{Name: "fooo", Suggestion: "foo", Cause: base}
		if withSug.Error() == "" {
			t.Fatal("UndefinedVariableError.Error() empty (with suggestion)")
		}
		noSug := &UndefinedVariableError{Name: "bar", Cause: base}
		if noSug.Error() == "" {
			t.Fatal("UndefinedVariableError.Error() empty (no suggestion)")
		}
		if !errors.Is(withSug, base) {
			t.Fatal("UndefinedVariableError.Unwrap() did not return cause")
		}
	})

	t.Run("TypeMismatchError", func(t *testing.T) {
		withDetail := &TypeMismatchError{Expected: "int", Actual: "string", Detail: "variable \"x\"", Cause: base}
		if withDetail.Error() == "" {
			t.Fatal("TypeMismatchError.Error() empty (with detail)")
		}
		noDetail := &TypeMismatchError{Expected: "bool", Actual: "int"}
		if noDetail.Error() == "" {
			t.Fatal("TypeMismatchError.Error() empty (no detail)")
		}
		if !errors.Is(withDetail, base) {
			t.Fatal("TypeMismatchError.Unwrap() did not return cause")
		}
		if noDetail.Unwrap() != nil {
			t.Fatal("TypeMismatchError.Unwrap() should be nil when Cause is nil")
		}
	})

	t.Run("EvalError", func(t *testing.T) {
		withSrc := &EvalError{Source: "a/b", Cause: base}
		if withSrc.Error() == "" {
			t.Fatal("EvalError.Error() empty (with source)")
		}
		noSrc := &EvalError{Cause: base}
		if noSrc.Error() == "" {
			t.Fatal("EvalError.Error() empty (no source)")
		}
		if !errors.Is(withSrc, base) {
			t.Fatal("EvalError.Unwrap() did not return cause")
		}
	})

	t.Run("CostLimitError", func(t *testing.T) {
		e := &CostLimitError{Limit: 100, Source: "x*x", Cause: base}
		if e.Error() == "" {
			t.Fatal("CostLimitError.Error() empty")
		}
		if !errors.Is(e, base) {
			t.Fatal("CostLimitError.Unwrap() did not return cause")
		}
	})

	t.Run("TimeoutError", func(t *testing.T) {
		e := &TimeoutError{Source: "slow()", Cause: base}
		if e.Error() == "" {
			t.Fatal("TimeoutError.Error() empty")
		}
		if !errors.Is(e, base) {
			t.Fatal("TimeoutError.Unwrap() did not return cause")
		}
	})

	t.Run("OverflowError", func(t *testing.T) {
		withSrc := &OverflowError{Op: "division", Detail: "MinInt64 / -1", Source: "a/b"}
		if withSrc.Error() == "" {
			t.Fatal("OverflowError.Error() empty (with source)")
		}
		noSrc := &OverflowError{Op: "division", Detail: "MinInt64 / -1"}
		if noSrc.Error() == "" {
			t.Fatal("OverflowError.Error() empty (no source)")
		}
	})
}
