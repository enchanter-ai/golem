package golem

import "time"

const (
	// defaultCacheSize is the LRU capacity used when WithCacheSize is not set
	// (or is set to a non-positive value).
	defaultCacheSize = 1024
)

// Option configures an Engine at construction time. Options are applied in
// order by New.
type Option func(*config)

// config is the resolved, internal Engine configuration. It is built up from
// Options and is never mutated after New returns, so it is safe to read
// concurrently.
type config struct {
	schema     Vars                  // declared variable schema (compile-time type-check via expr.Env)
	functions  map[string]customFunc // host-registered functions, by name
	mathStdlib bool                  // enable the curated math.* allowlist
	strictVars bool                  // true: undeclared identifier = compile error; false: resolves to nil
	cacheSize  int                   // LRU program-cache capacity
	timeout    time.Duration         // per-Eval wall-clock deadline (0 = none)
	costLimit  int                   // AST node-visit budget (0 = unlimited)
}

// customFunc holds a host-registered function and its name. fn is the raw value
// supplied to WithFunction; functions.go adapts it to expr's signature.
type customFunc struct {
	name string
	fn   any
}

// defaultConfig returns the baseline configuration: strict variables ON (LOUD
// by default), the default cache size, no math stdlib, no limits.
func defaultConfig() *config {
	return &config{
		functions:  make(map[string]customFunc),
		strictVars: true,
		cacheSize:  defaultCacheSize,
	}
}

// WithVariables declares the variable schema. golem compiles each expression
// against this schema via expr.Env, so an undeclared top-level identifier
// becomes a compile-time UndefinedVariableError (the LOUD contract) and a
// type-incompatible operation becomes a TypeMismatchError. The declared types
// also drive numeric input normalization at Eval time.
func WithVariables(schema Vars) Option {
	return func(c *config) { c.schema = schema }
}

// WithFunction registers a host function callable by name from expression text
// (e.g. WithFunction("clamp", clampFn) lets an expression call clamp(x,0,1)).
// The function is wired into expr.Function at both compile and run time. fn may
// be any Go func; it runs with full Go permissions (v1 does not sandbox custom
// functions beyond the panic boundary).
func WithFunction(name string, fn any) Option {
	return func(c *config) {
		c.functions[name] = customFunc{name: name, fn: fn}
	}
}

// WithMathStdlib enables a curated allowlist of math.* functions (sqrt, abs,
// floor, ceil, round, pow, min, max, ...) inside expressions. expr provides the
// implementations; golem only opts them in.
func WithMathStdlib() Option {
	return func(c *config) { c.mathStdlib = true }
}

// WithStrictVars controls the null/undefined policy. The default is true: an
// undeclared top-level identifier is a compile-time UndefinedVariableError.
// Setting it false enables expr's AllowUndefinedVariables, so undeclared
// variables resolve to nil at runtime instead of failing to compile.
func WithStrictVars(strict bool) Option {
	return func(c *config) { c.strictVars = strict }
}

// WithCacheSize sets the per-Engine LRU program-cache capacity. A non-positive
// value falls back to the default.
func WithCacheSize(n int) Option {
	return func(c *config) { c.cacheSize = n }
}

// WithEvalTimeout sets a per-Eval wall-clock deadline.
//
// The timeout is COOPERATIVE, not preemptive. expr has no VM-level preemption:
// golem enforces the deadline by racing expr.Run in a worker goroutine against
// the deadline and returning TimeoutError if the deadline wins. The abandoned
// worker goroutine keeps running until the expression (or the custom function
// it is blocked in) returns on its own — the deadline only bounds how long the
// caller waits, it does NOT abort in-flight work. A genuinely runaway
// expression therefore leaks a goroutine until it finishes.
//
// To get a real bound, combine two guards:
//   - WithCostLimit as the primary production guard (a hard, compile/run-time
//     AST-node budget that expr enforces deterministically); and
//   - custom functions that honor the ctx passed via expr.WithContext("ctx") —
//     poll ctx.Done() in any long-running loop and return early on cancellation.
//
// Prefer WithCostLimit as the primary guard; treat WithEvalTimeout as a
// caller-wait bound layered on top of cooperative functions.
func WithEvalTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithCostLimit sets the AST node-visit budget (expr.MaxNodes). An expression
// whose node count exceeds n fails to compile with a CostLimitError. The
// default is unlimited. This is the preferred production guard against
// pathological expressions.
func WithCostLimit(n int) Option {
	return func(c *config) { c.costLimit = n }
}
