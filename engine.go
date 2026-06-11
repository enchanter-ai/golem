package golem

import (
	"fmt"
	"math"
	"strings"

	"github.com/expr-lang/expr"
)

// Vars is a set of named variables supplied to an evaluation, or a declared
// variable schema (WithVariables). Values are plain Go types (bool, numeric,
// string, slices/maps for structured data).
type Vars map[string]any

// Engine is a thread-safe, reusable expression engine. It owns a bounded LRU
// cache of compiled programs and a frozen configuration (schema, functions,
// limits, null policy). Build one with New and share it across goroutines; its
// safety comes from the cache's own locking — golem adds no extra mutex.
type Engine struct {
	cfg   *config
	cache *programCache
	// compileOpts is the frozen expr.Option set (Env, functions, limits, null
	// policy). Built once in New so Compile is allocation-light and the compile
	// and run passes share an identical function/option set.
	compileOpts []expr.Option
	// initErr, when non-nil, marks an Engine that was mis-configured at
	// construction via NewEngineJSON (the gopy binding boundary). EvalJSON
	// surfaces it as a LOUD failure envelope on first use rather than letting a
	// silently mis-configured engine run. The Go-native constructors never set
	// it.
	initErr error
}

// New constructs an Engine from the given options. It is LOUD by default:
// without WithStrictVars(false), an undeclared top-level identifier is a
// compile-time UndefinedVariableError.
func New(opts ...Option) *Engine {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	cache, err := newProgramCache(cfg.cacheSize)
	if err != nil {
		// lru.New only errors on a non-positive size, which newProgramCache
		// already guards; fall back to an unbounded-by-default safe size.
		cache, _ = newProgramCache(defaultCacheSize)
	}

	e := &Engine{cfg: cfg, cache: cache}
	e.compileOpts = e.buildCompileOptions()
	return e
}

// buildCompileOptions assembles the expr options shared by every Compile call:
// the declared Env, the null policy, custom functions, math stdlib opt-in, and
// the cost (node-budget) limit. The same function options are reused at run
// time because expr.Run inherits them from the compiled program.
func (e *Engine) buildCompileOptions() []expr.Option {
	opts := make([]expr.Option, 0, 8)

	if e.cfg.schema != nil {
		opts = append(opts, expr.Env(map[string]any(e.cfg.schema)))
	} else {
		// No declared schema: still pin an (empty) Env so expr's type checker
		// stays in strict mode and an unknown identifier or function call (e.g.
		// sqrt without WithMathStdlib) is a compile-time UndefinedVariableError.
		// Without any Env, expr is permissive about unknown names, which would
		// silently break the LOUD contract. When strict vars are off we add
		// AllowUndefinedVariables below to relax this again.
		opts = append(opts, expr.Env(map[string]any{}))
	}
	if !e.cfg.strictVars {
		opts = append(opts, expr.AllowUndefinedVariables())
	}

	// Integer-division semantics: expr's native "/" is always float division
	// (5/2 == 2.5) and integer division-by-zero yields +Inf rather than a Go
	// runtime panic. golem overrides "/" with golem_div so that int/int uses
	// Go's truncating integer division (5/2 == 2) and int-by-zero panics into
	// the run-time panic boundary (-> EvalError). Any operand that is a float
	// keeps float division. The typed witnesses let expr's checker resolve the
	// operator at the int/int and float/float call sites.
	opts = append(opts,
		expr.Function("golem_div", golemDiv, divIntWitness, divInt64Witness, divFloatWitness),
		expr.Operator("/", "golem_div"),
	)
	if e.cfg.costLimit > 0 {
		opts = append(opts, expr.MaxNodes(uint(e.cfg.costLimit))) //nolint:gosec // G115: costLimit > 0 enforced by the guard above
	}
	if e.cfg.timeout > 0 {
		// Expose a context to cooperative custom functions; does NOT preempt
		// the VM (see Program.runWithTimeout).
		opts = append(opts, expr.WithContext("ctx"))
	}

	fnOpts, err := functionOptions(e.cfg)
	if err == nil {
		opts = append(opts, fnOpts...)
	}
	// Function build errors surface at Compile time via a re-run below; storing
	// them here would complicate New's signature. They are deterministic in the
	// config, so Compile re-validates and returns them as the caller's error.

	return opts
}

// Compile parses and type-checks src once, caching the resulting program per
// Engine (keyed by source). Subsequent Compile/Eval of the same source reuse
// the cached program. Errors are classified into golem's typed-error set;
// no expr-internal type leaks.
func (e *Engine) Compile(src string) (*Program, error) {
	if prog, ok := e.cache.get(src); ok {
		return &Program{source: src, program: prog, cfg: e.cfg}, nil
	}

	// Validate custom-function definitions deterministically (returns a typed
	// or descriptive error rather than silently dropping a function).
	if _, ferr := functionOptions(e.cfg); ferr != nil {
		return nil, &EvalError{Source: src, Cause: ferr}
	}

	prog, err := expr.Compile(src, e.compileOpts...)
	if err != nil {
		return nil, classifyCompileError(src, err, e.cfg)
	}

	e.cache.add(src, prog)
	return &Program{source: src, program: prog, cfg: e.cfg}, nil
}

// Eval compiles (with caching) and runs src against vars in one call.
func (e *Engine) Eval(src string, vars Vars) (Value, error) {
	p, err := e.Compile(src)
	if err != nil {
		return Value{}, err
	}
	return p.Eval(vars)
}

// classifyCompileError maps an expr compile-time error onto golem's typed-error
// set by inspecting its message. expr surfaces:
//   - "unknown name X"            -> UndefinedVariableError (with a suggestion)
//   - "unexpected token ..."      -> ParseError
//   - "... exceeds ... nodes"     -> CostLimitError (MaxNodes budget)
//   - "mismatched types" / "invalid operation" / "cannot use" -> TypeMismatchError
//
// Anything else falls back to ParseError, the safest compile-time bucket.
func classifyCompileError(src string, err error, cfg *config) error {
	msg := err.Error()
	low := strings.ToLower(msg)

	switch {
	case strings.Contains(low, "unknown name"):
		name := extractUnknownName(msg)
		return &UndefinedVariableError{
			Name:       name,
			Suggestion: suggestName(name, cfg.schema),
			Cause:      err,
		}
	case cfg.costLimit > 0 && (strings.Contains(low, "exceeds") || strings.Contains(low, "too many nodes") || strings.Contains(low, "node")):
		return &CostLimitError{Limit: cfg.costLimit, Cause: err, Source: src}
	case strings.Contains(low, "mismatched types"),
		strings.Contains(low, "invalid operation"),
		strings.Contains(low, "cannot use"),
		strings.Contains(low, "expected type"):
		return &TypeMismatchError{Expected: "compatible types", Actual: "mismatched", Detail: firstLine(msg)}
	case strings.Contains(low, "unexpected"),
		strings.Contains(low, "unclosed"),
		strings.Contains(low, "literal not terminated"),
		strings.Contains(low, "expected"):
		return &ParseError{Source: src, Cause: err}
	default:
		return &ParseError{Source: src, Cause: err}
	}
}

// classifyRunError maps an expr runtime error onto golem's typed-error set.
// Runtime failures (division by zero, nil deref not masked by ?., etc.) become
// EvalError unless the message indicates the node budget was exceeded.
func classifyRunError(src string, err error) error {
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "exceeds") && strings.Contains(low, "node") {
		return &CostLimitError{Cause: err, Source: src}
	}
	return &EvalError{Source: src, Cause: err}
}

// extractUnknownName pulls the identifier out of expr's "unknown name X (l:c)"
// message.
func extractUnknownName(msg string) string {
	const marker = "unknown name "
	i := strings.Index(msg, marker)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(marker):]
	// The name runs up to the first space or '(' that begins the location.
	end := strings.IndexAny(rest, " (\n")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// firstLine returns the first line of a possibly multi-line expr error message,
// keeping golem's error messages compact and free of expr's source snippets.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// suggestName returns the declared variable whose name is closest to the given
// (undeclared) identifier, for the "did you mean" hint, or "" if nothing is
// within a small edit distance.
func suggestName(name string, schema Vars) string {
	if name == "" || len(schema) == 0 {
		return ""
	}
	best := ""
	bestDist := len(name)/2 + 1 // tolerate up to ~half the length in edits
	for candidate := range schema {
		d := levenshtein(name, candidate)
		if d < bestDist {
			bestDist = d
			best = candidate
		}
	}
	return best
}

// levenshtein computes the edit distance between two short identifiers.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// divIntWitness, divInt64Witness, and divFloatWitness are type witnesses for the
// "/" operator override: they tell expr's checker the resolved call shapes
// (int/int, int64/int64, and float/float) without ever being invoked at run
// time. The int64 witness is required so that int64-typed operands route through
// golem_div (truncating integer division) instead of falling back to expr's
// native float "/".
func divIntWitness(a, b int) int           { return a / b }
func divInt64Witness(a, b int64) int64     { return a / b }
func divFloatWitness(a, b float64) float64 { return a / b }

// golemDiv implements golem's division semantics for the overridden "/"
// operator. When both operands are integers it uses Go's truncating integer
// division (5/2 == 2); an int divide-by-zero panics here and is converted to
// an EvalError by the run-time panic boundary (Program.safeRun). If either
// operand is a float the result is float division. Mixed/unknown operand types
// fall back to float division after coercion, matching expr's promotion rules.
func golemDiv(params ...any) (any, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("golem: division expects 2 operands, got %d", len(params))
	}
	a, b := params[0], params[1]
	ai, aIsInt := toInt(a)
	bi, bIsInt := toInt(b)
	if aIsInt && bIsInt {
		// Truncating integer division for ALL integer kinds (int, int64, int32,
		// ...) since toInt normalizes them to int64. We guard the two
		// failure cases explicitly rather than relying on a Go runtime panic:
		//   - divide-by-zero -> typed EvalError
		//   - math.MinInt64 / -1 -> OverflowError, which would otherwise
		//     silently wrap back to math.MinInt64.
		if bi == 0 {
			return nil, &EvalError{Cause: fmt.Errorf("golem: integer division by zero")}
		}
		if ai == math.MinInt64 && bi == -1 {
			return nil, &OverflowError{
				Op:     "division",
				Detail: "math.MinInt64 / -1 is not representable in int64",
			}
		}
		return ai / bi, nil
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return nil, &TypeMismatchError{
			Expected: "numeric / numeric",
			Actual:   typeName(a) + " / " + typeName(b),
		}
	}
	return af / bf, nil
}

// toInt reports whether v is an integer kind (not a float) and its int64 value.
func toInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int16:
		return int64(n), true
	case int8:
		return int64(n), true
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true //nolint:gosec // G115: range-checked immediately above
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true //nolint:gosec // G115: range-checked immediately above
	case uint32:
		return int64(n), true
	default:
		return 0, false
	}
}

// toFloat coerces any numeric value to float64 for float division.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case int16:
		return float64(n), true
	case int8:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	default:
		return 0, false
	}
}
