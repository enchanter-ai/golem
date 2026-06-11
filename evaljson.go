package golem

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// engineOptionsJSON is the wire form of an Engine configuration for the gopy
// Python binding. Custom functions are intentionally absent: the binding cannot
// marshal Go funcs, and the Python Engine does not accept Python callables in v1
// (see python/golem). Numeric schema values arrive as JSON numbers, normalized
// to int64/float64 just like the Eval var path.
type engineOptionsJSON struct {
	Variables  map[string]any `json:"variables,omitempty"`
	StrictVars *bool          `json:"strict_vars,omitempty"`
	MathStdlib bool           `json:"math_stdlib,omitempty"`
	CacheSize  int            `json:"cache_size,omitempty"`
	TimeoutMS  int64          `json:"timeout_ms,omitempty"`
	CostLimit  int            `json:"cost_limit,omitempty"`
}

// NewEngineJSON constructs an Engine from a JSON options object. It is the
// symmetric companion of EvalJSON for the gopy Python binding: gopy marshals
// variadic functional Options and map[string]any poorly, so the binding passes
// engine configuration as a JSON string. Custom functions cannot cross this
// boundary (the Python binding does not support Python callables in v1).
//
// Like EvalJSON it NEVER returns a Go error across the gopy boundary
// (go-python/gopy issue #254): a malformed options string yields an Engine that
// fails LOUD on first EvalJSON with an EvalError envelope, rather than a
// silently mis-configured (schema-less / lenient) engine — that would defeat the
// LOUD contract. An empty string returns a default (zero-config, LOUD) Engine.
//
// Recognized fields: {"variables":{...}, "strict_vars":bool, "math_stdlib":bool,
// "cache_size":int, "timeout_ms":int, "cost_limit":int}.
func NewEngineJSON(optionsJSON string) *Engine {
	if strings.TrimSpace(optionsJSON) == "" {
		return New()
	}
	var o engineOptionsJSON
	dec := json.NewDecoder(strings.NewReader(optionsJSON))
	dec.UseNumber()
	if err := dec.Decode(&o); err != nil {
		e := New()
		e.initErr = &EvalError{Cause: fmt.Errorf("golem: invalid engine options JSON: %w", err)}
		return e
	}

	opts := make([]Option, 0, 6)
	if len(o.Variables) > 0 {
		schema := make(Vars, len(o.Variables))
		for k, v := range o.Variables {
			schema[k] = normalizeJSONNumber(v)
		}
		opts = append(opts, WithVariables(schema))
	}
	if o.StrictVars != nil {
		opts = append(opts, WithStrictVars(*o.StrictVars))
	}
	if o.MathStdlib {
		opts = append(opts, WithMathStdlib())
	}
	if o.CacheSize > 0 {
		opts = append(opts, WithCacheSize(o.CacheSize))
	}
	if o.TimeoutMS > 0 {
		opts = append(opts, WithEvalTimeout(time.Duration(o.TimeoutMS)*time.Millisecond))
	}
	if o.CostLimit > 0 {
		opts = append(opts, WithCostLimit(o.CostLimit))
	}
	return New(opts...)
}

// EvalJSON is the binding-only boundary used by the gopy-generated Python
// extension. It accepts the expression source and a JSON object of variables,
// and returns a SINGLE JSON envelope string. It NEVER returns a Go error across
// the boundary — gopy's (value, error) -> Python exception mapping is buggy for
// (string, error) (go-python/gopy issue #254), so every outcome, success or
// failure, is encoded inside the envelope. The thin Python wrapper parses it,
// restores the Python type from the "type" tag, and raises the mapped exception
// when ok=false.
//
// Success: {"ok":true,"type":"int|float|bool|string|null","value":<json>}
// Failure: {"ok":false,"errtype":"<TypedErrorName>","error":"<message>"}
//
// Encoding rules:
//   - NaN/Inf results          -> failure EvalError (JSON cannot encode them).
//   - collection (slice/map)   -> failure EvalError "collection results
//     unsupported via the Python binding in v1".
//   - a float that is integral (e.g. 14.0) still carries type:"float" so the
//     Python side reconstructs a Python float, not an int.
func (e *Engine) EvalJSON(src, varsJSON string) string {
	if e.initErr != nil {
		return failureEnvelope(errType(e.initErr), e.initErr.Error())
	}
	vars, err := decodeVarsJSON(varsJSON)
	if err != nil {
		return failureEnvelope("EvalError", "invalid variables JSON: "+err.Error())
	}

	v, evErr := e.Eval(src, vars)
	if evErr != nil {
		return failureEnvelope(errType(evErr), evErr.Error())
	}

	return encodeResultEnvelope(v.AsAny())
}

// decodeVarsJSON parses the variables object. An empty string is treated as an
// empty variable set.
func decodeVarsJSON(varsJSON string) (Vars, error) {
	if varsJSON == "" {
		return Vars{}, nil
	}
	var raw map[string]any
	dec := json.NewDecoder(strings.NewReader(varsJSON))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	// Convert json.Number to int64 when integral, else float64, so the numeric
	// model and schema normalization behave identically to the Go-native path.
	vars := make(Vars, len(raw))
	for k, val := range raw {
		vars[k] = normalizeJSONNumber(val)
	}
	return vars, nil
}

// normalizeJSONNumber recursively turns json.Number into int64/float64.
func normalizeJSONNumber(v any) any {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n.String()
	case map[string]any:
		for k, e := range n {
			n[k] = normalizeJSONNumber(e)
		}
		return n
	case []any:
		for i, e := range n {
			n[i] = normalizeJSONNumber(e)
		}
		return n
	default:
		return v
	}
}

// envelope mirrors the success/failure JSON shapes for marshalling.
type envelope struct {
	OK      bool   `json:"ok"`
	Type    string `json:"type,omitempty"`
	Value   any    `json:"value,omitempty"`
	ErrType string `json:"errtype,omitempty"`
	Error   string `json:"error,omitempty"`
}

// encodeResultEnvelope encodes a successful result, applying the type tagging
// and the NaN/Inf and collection rules.
func encodeResultEnvelope(raw any) string {
	switch r := raw.(type) {
	case nil:
		return marshalEnvelope(envelope{OK: true, Type: "null", Value: nil})
	case bool:
		return marshalEnvelope(envelope{OK: true, Type: "bool", Value: r})
	case string:
		return marshalEnvelope(envelope{OK: true, Type: "string", Value: r})
	case float64:
		if math.IsNaN(r) || math.IsInf(r, 0) {
			return failureEnvelope("EvalError", "result is NaN or Inf, which JSON cannot encode")
		}
		return marshalEnvelope(envelope{OK: true, Type: "float", Value: r})
	case float32:
		f := float64(r)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return failureEnvelope("EvalError", "result is NaN or Inf, which JSON cannot encode")
		}
		return marshalEnvelope(envelope{OK: true, Type: "float", Value: f})
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return marshalEnvelope(envelope{OK: true, Type: "int", Value: r})
	default:
		// Slices, maps, structs, and any other composite: unsupported via the
		// v1 Python binding.
		return failureEnvelope("EvalError", "collection results unsupported via the Python binding in v1")
	}
}

// failureEnvelope builds a failure JSON envelope. It uses a hand-built JSON
// fallback if marshalling somehow fails, so the boundary never panics or
// returns a Go error.
func failureEnvelope(errType, msg string) string {
	return marshalEnvelope(envelope{OK: false, ErrType: errType, Error: msg})
}

// marshalEnvelope serializes an envelope; on the (practically impossible)
// marshal failure it returns a static, valid failure envelope.
func marshalEnvelope(env envelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return `{"ok":false,"errtype":"EvalError","error":"failed to encode result envelope"}`
	}
	return string(b)
}

// errType returns the typed-error name for an error from the typed-error set,
// for the envelope's errtype field. Unknown errors map to EvalError.
func errType(err error) string {
	switch err.(type) {
	case *ParseError:
		return "ParseError"
	case *UndefinedVariableError:
		return "UndefinedVariableError"
	case *TypeMismatchError:
		return "TypeMismatchError"
	case *CostLimitError:
		return "CostLimitError"
	case *TimeoutError:
		return "TimeoutError"
	case *OverflowError:
		return "OverflowError"
	case *EvalError:
		return "EvalError"
	default:
		return "EvalError"
	}
}
