package golem

import "fmt"

// The typed-error set. Every failure golem surfaces is one of these concrete
// types, so callers can switch on them (errors.As) and the Python binding can
// map each to a same-named exception. No expr-internal type leaks through any
// of these messages or fields.

// ParseError reports a malformed or syntactically invalid expression. It is
// produced at compile time.
type ParseError struct {
	Source string // the offending expression source
	Cause  error  // underlying parse failure (message only; no expr type leaks)
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("golem: parse error in %q: %s", e.Source, e.Cause)
}

func (e *ParseError) Unwrap() error { return e.Cause }

// UndefinedVariableError reports an undeclared top-level identifier referenced
// by an expression while strict-variable mode is on (the default). It carries
// the offending name and, when one is available, a "did you mean" suggestion
// drawn from the declared schema.
type UndefinedVariableError struct {
	Name       string // the undeclared identifier
	Suggestion string // closest declared name, or "" if none is close
	Cause      error  // underlying compile failure
}

func (e *UndefinedVariableError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("golem: undefined variable %q (did you mean %q?)", e.Name, e.Suggestion)
	}
	return fmt.Sprintf("golem: undefined variable %q", e.Name)
}

func (e *UndefinedVariableError) Unwrap() error { return e.Cause }

// TypeMismatchError reports a type-check failure: either a compile-time type
// error from the declared schema, an input variable whose type cannot be
// coerced to its declared slot, or a typed accessor (Value.As*/Program.Eval*)
// asked for a type the result is not.
type TypeMismatchError struct {
	Expected string // the type that was expected (e.g. "float64", "bool")
	Actual   string // the type that was found (e.g. "string", "int")
	Detail   string // optional human context (no expr type leaks)
	Cause    error  // optional underlying cause; nil when none applies
}

func (e *TypeMismatchError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("golem: type mismatch: %s (expected %s, got %s)", e.Detail, e.Expected, e.Actual)
	}
	return fmt.Sprintf("golem: type mismatch: expected %s, got %s", e.Expected, e.Actual)
}

func (e *TypeMismatchError) Unwrap() error { return e.Cause }

// EvalError reports a runtime failure during evaluation, including any panic
// recovered at the safeRun boundary. The original cause is always preserved.
type EvalError struct {
	Source string // the expression source, if known
	Cause  error  // underlying runtime failure or recovered panic value
}

func (e *EvalError) Error() string {
	if e.Source != "" {
		return fmt.Sprintf("golem: eval error in %q: %s", e.Source, e.Cause)
	}
	return fmt.Sprintf("golem: eval error: %s", e.Cause)
}

func (e *EvalError) Unwrap() error { return e.Cause }

// CostLimitError reports that an expression exceeded the configured
// AST-node-visit budget (WithCostLimit). It is the preferred production guard
// against pathological expressions.
type CostLimitError struct {
	Limit  int    // the configured node-visit budget
	Cause  error  // underlying compile/run failure
	Source string // the expression source, if known
}

func (e *CostLimitError) Error() string {
	return fmt.Sprintf("golem: cost limit exceeded (budget %d) for %q", e.Limit, e.Source)
}

func (e *CostLimitError) Unwrap() error { return e.Cause }

// TimeoutError reports that an evaluation exceeded the deadline configured by
// WithEvalTimeout. Because expr has no VM-level preemption, the abandoned
// evaluation goroutine may keep running until the expression (or a custom
// function) returns; the timeout only bounds how long the caller waits.
type TimeoutError struct {
	Source string // the expression source, if known
	Cause  error  // optional underlying cause (e.g. context deadline); nil when none applies
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("golem: evaluation timed out for %q", e.Source)
}

func (e *TimeoutError) Unwrap() error { return e.Cause }

// OverflowError reports an integer-arithmetic operation whose mathematically
// correct result is not representable in int64 — currently the single
// two's-complement edge case math.MinInt64 / -1, which would silently wrap to
// math.MinInt64 under Go's division. golem surfaces it as a typed error rather
// than returning the wrapped value.
type OverflowError struct {
	Op     string // the operation, e.g. "division" or "conversion"
	Detail string // human context, e.g. "math.MinInt64 / -1 overflows int64"
	Source string // the expression source, if known
	Cause  error  // optional underlying cause; nil when none applies
}

func (e *OverflowError) Error() string {
	if e.Source != "" {
		return fmt.Sprintf("golem: integer %s overflow in %q: %s", e.Op, e.Source, e.Detail)
	}
	return fmt.Sprintf("golem: integer %s overflow: %s", e.Op, e.Detail)
}

func (e *OverflowError) Unwrap() error { return e.Cause }
