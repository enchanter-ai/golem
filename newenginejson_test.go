package golem

import (
	"encoding/json"
	"testing"
)

// decodeEnvelope unmarshals an EvalJSON/NewEngineJSON envelope for assertions.
func decodeEnvelope(t *testing.T, s string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v (%q)", err, s)
	}
	return env
}

// NewEngineJSON is the gopy-binding boundary: it must build a configured engine
// from a JSON options string without ever returning a Go error, and preserve the
// LOUD contract (declared schema -> typo is a compile-time error envelope).
func TestNewEngineJSON_SchemaEvaluatesAndIsLoud(t *testing.T) {
	e := NewEngineJSON(`{"variables":{"x":0.0}}`)

	// Happy path: the flagship expression, x supplied via the vars envelope.
	got := decodeEnvelope(t, e.EvalJSON("2 + 3 * (x - 1)", `{"x":5}`))
	if !got.OK {
		t.Fatalf("expected ok envelope, got %+v", got)
	}
	if got.Type != "float" {
		t.Fatalf("type = %q, want float (x declared float64)", got.Type)
	}

	// LOUD: a typo'd identifier against the declared schema is a compile-time
	// UndefinedVariableError surfaced through the failure envelope.
	bad := decodeEnvelope(t, e.EvalJSON("revenu * 2", `{}`))
	if bad.OK {
		t.Fatalf("expected failure envelope for undeclared 'revenu', got ok")
	}
	if bad.ErrType != "UndefinedVariableError" {
		t.Fatalf("errtype = %q, want UndefinedVariableError", bad.ErrType)
	}
}

// strict_vars:false must relax the schema to expr's AllowUndefinedVariables so an
// undeclared identifier resolves to nil instead of failing — proving the flag
// crosses the JSON boundary.
func TestNewEngineJSON_LenientFlag(t *testing.T) {
	e := NewEngineJSON(`{"strict_vars":false}`)
	got := decodeEnvelope(t, e.EvalJSON(`missing ?? "fallback"`, `{}`))
	if !got.OK {
		t.Fatalf("lenient engine should evaluate, got failure %+v", got)
	}
	if got.Type != "string" || got.Value != "fallback" {
		t.Fatalf("got type=%q value=%v, want string/fallback", got.Type, got.Value)
	}
}

// An empty options string yields a default (zero-config, LOUD) engine.
func TestNewEngineJSON_EmptyIsDefault(t *testing.T) {
	e := NewEngineJSON("")
	got := decodeEnvelope(t, e.EvalJSON("1 + 1", `{}`))
	if !got.OK || got.Type != "int" {
		t.Fatalf("empty-options engine failed: %+v", got)
	}
}

// Malformed options JSON must NOT panic or return a Go error; it must defer a
// LOUD EvalError to the first EvalJSON (never a silently mis-configured engine).
func TestNewEngineJSON_MalformedDefersLoudError(t *testing.T) {
	e := NewEngineJSON(`{"variables": this is not json`)
	got := decodeEnvelope(t, e.EvalJSON("1 + 1", `{}`))
	if got.OK {
		t.Fatalf("malformed options should fail loud, got ok envelope")
	}
	if got.ErrType != "EvalError" {
		t.Fatalf("errtype = %q, want EvalError", got.ErrType)
	}
}
