package golem

import (
	"fmt"
	"math"
)

// Value is the engine-agnostic result of an evaluation. It exposes only
// primitive Go types (bool, float64, int64, string) plus the raw any; no
// expr-internal type is ever stored in or returned from a Value.
//
// Numeric model: expr returns a Go int for integer arithmetic and a float64
// for float arithmetic. AsFloat WIDENS an int/int64 result to float64 — this is
// the ONLY implicit result coercion golem performs, and it is documented.
// AsInt returns a TypeMismatchError on a true (non-integral) float.
type Value struct {
	raw any
}

// newValue wraps a raw evaluation result. The raw value originates from expr's
// VM, but only Go primitives (or nil) ever reach a caller through the accessors
// below; the raw any is exposed solely via AsAny for advanced callers.
func newValue(raw any) Value { return Value{raw: raw} }

// AsAny returns the raw underlying result (bool, int/int64, float64, string,
// nil, or — for collection-returning expressions — a slice/map). Callers that
// need a guaranteed primitive should prefer the typed accessors.
func (v Value) AsAny() any { return v.raw }

// IsNil reports whether the result is nil (e.g. a lenient-mode undefined lookup
// or a `?.`/`??` chain that resolved to null).
func (v Value) IsNil() bool { return v.raw == nil }

// AsBool returns the result as a bool, or a TypeMismatchError if it is not a
// bool.
func (v Value) AsBool() (bool, error) {
	if b, ok := v.raw.(bool); ok {
		return b, nil
	}
	return false, &TypeMismatchError{Expected: "bool", Actual: typeName(v.raw)}
}

// AsString returns the result as a string, or a TypeMismatchError if it is not
// a string.
func (v Value) AsString() (string, error) {
	if s, ok := v.raw.(string); ok {
		return s, nil
	}
	return "", &TypeMismatchError{Expected: "string", Actual: typeName(v.raw)}
}

// AsFloat returns the result as a float64. Integer results (int / int64 and the
// other Go integer kinds expr may produce) are WIDENED to float64 — the only
// implicit result coercion golem performs. A true float passes through. Any
// non-numeric result yields a TypeMismatchError.
func (v Value) AsFloat() (float64, error) {
	switch n := v.raw.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	default:
		return 0, &TypeMismatchError{Expected: "float64", Actual: typeName(v.raw)}
	}
}

// AsInt returns the result as an int64. Integer results pass through, except an
// unsigned value above math.MaxInt64, which yields an OverflowError rather than
// a silently wrapped negative. A float result is accepted ONLY if it is exactly
// integral (e.g. 14.0); a true float (e.g. 2.5) yields a TypeMismatchError.
// Non-numeric results yield a TypeMismatchError.
func (v Value) AsInt() (int64, error) {
	switch n := v.raw.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int16:
		return int64(n), nil
	case int8:
		return int64(n), nil
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, &OverflowError{Op: "conversion", Detail: fmt.Sprintf("uint value %d exceeds int64 range", n)}
		}
		return int64(n), nil //nolint:gosec // G115: range-checked immediately above
	case uint64:
		if n > math.MaxInt64 {
			return 0, &OverflowError{Op: "conversion", Detail: fmt.Sprintf("uint64 value %d exceeds int64 range", n)}
		}
		return int64(n), nil //nolint:gosec // G115: range-checked immediately above
	case uint32:
		return int64(n), nil
	case uint16:
		return int64(n), nil
	case uint8:
		return int64(n), nil
	case float64:
		if n == float64(int64(n)) {
			return int64(n), nil
		}
		return 0, &TypeMismatchError{Expected: "int", Actual: "float64", Detail: fmt.Sprintf("non-integral value %v", n)}
	case float32:
		if float64(n) == float64(int64(n)) {
			return int64(n), nil
		}
		return 0, &TypeMismatchError{Expected: "int", Actual: "float32", Detail: fmt.Sprintf("non-integral value %v", n)}
	default:
		return 0, &TypeMismatchError{Expected: "int", Actual: typeName(v.raw)}
	}
}

// typeName returns a stable, expr-free type label for error messages.
func typeName(x any) string {
	if x == nil {
		return "nil"
	}
	switch x.(type) {
	case bool:
		return "bool"
	case string:
		return "string"
	case float64, float32:
		return "float"
	case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return "int"
	default:
		return fmt.Sprintf("%T", x)
	}
}
