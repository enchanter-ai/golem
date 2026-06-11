package golem

import (
	"context"
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Program is an immutable, compiled expression. expr's *vm.Program is
// thread-safe for reuse, so a Program may be evaluated concurrently from many
// goroutines without external synchronization.
type Program struct {
	source  string
	program *vm.Program
	cfg     *config // shared, never mutated after New
}

// Source returns the original expression text the Program was compiled from.
func (p *Program) Source() string { return p.source }

// Eval runs the compiled program against vars and returns the typed result.
// Input numeric vars are normalized to the declared schema type before the run
// (see normalizeVars). Any panic inside expr or a custom function is recovered
// at the boundary and returned as an EvalError — never propagated. If the
// Engine has an eval timeout, the run is raced against the deadline.
func (p *Program) Eval(vars Vars) (Value, error) {
	env, err := p.buildEnv(vars)
	if err != nil {
		return Value{}, err
	}
	raw, err := p.run(env)
	if err != nil {
		return Value{}, err
	}
	return newValue(raw), nil
}

// EvalBool evaluates and returns a bool, or a TypeMismatchError if the result
// is not a bool.
func (p *Program) EvalBool(vars Vars) (bool, error) {
	v, err := p.Eval(vars)
	if err != nil {
		return false, err
	}
	return v.AsBool()
}

// EvalFloat evaluates and returns a float64, widening integer results per the
// numeric model. Non-numeric results yield a TypeMismatchError.
func (p *Program) EvalFloat(vars Vars) (float64, error) {
	v, err := p.Eval(vars)
	if err != nil {
		return 0, err
	}
	return v.AsFloat()
}

// EvalInt evaluates and returns an int64. A true (non-integral) float result
// yields a TypeMismatchError.
func (p *Program) EvalInt(vars Vars) (int64, error) {
	v, err := p.Eval(vars)
	if err != nil {
		return 0, err
	}
	return v.AsInt()
}

// EvalString evaluates and returns a string, or a TypeMismatchError if the
// result is not a string.
func (p *Program) EvalString(vars Vars) (string, error) {
	v, err := p.Eval(vars)
	if err != nil {
		return "", err
	}
	return v.AsString()
}

// buildEnv assembles the runtime environment map from vars, normalizing numeric
// inputs to the declared schema type. With a configured schema golem starts
// from the declared keys so the run env shape matches the compile-time Env.
func (p *Program) buildEnv(vars Vars) (map[string]any, error) {
	env := make(map[string]any, len(vars)+len(p.cfg.schema))
	for k, v := range vars {
		env[k] = v
	}
	if err := normalizeVars(env, p.cfg.schema); err != nil {
		return nil, err
	}
	return env, nil
}

// run executes the program against env inside a worker goroutine, applying the
// panic boundary and (if configured) the timeout race. Every evaluation — with
// or without a timeout — runs in a worker so that a custom function calling
// runtime.Goexit() cannot unwind THROUGH the caller's stack and hang Eval. A
// Goexit unwinds the worker's deferred recover (recover returns nil for Goexit),
// so safeRun never sends on the channel; the missing 'completed' sentinel is
// detected and returned as a typed EvalError instead of blocking forever.
func (p *Program) run(env map[string]any) (any, error) {
	if p.cfg.timeout <= 0 {
		return p.runWorker(env, nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.timeout)
	defer cancel()
	// Make the deadline ctx available to cooperative custom functions. NOTE:
	// the timeout is COOPERATIVE — see runWorker / WithEvalTimeout.
	env["ctx"] = ctx
	return p.runWorker(env, ctx)
}

// runResult carries a single run outcome across the goroutine boundary. The
// completed flag is the Goexit sentinel: a normal (or panicking-and-recovered)
// run sets completed=true before sending; a runtime.Goexit in a custom function
// unwinds the worker past the send, so the channel is closed without completed
// ever being observed. The reader treats a closed-without-result channel as a
// Goexit and returns an EvalError.
type runResult struct {
	value     any
	err       error
	completed bool
}

// runWorker runs safeRun in a worker goroutine and waits for either its result,
// the (optional) deadline, or worker abandonment via runtime.Goexit.
//
// The timeout, when ctx != nil, is COOPERATIVE: expr has NO VM-level
// preemption (expr.WithContext only feeds a ctx into custom functions, it
// cannot abort a running Run). On deadline we return TimeoutError but the
// abandoned worker keeps running until the expression (or custom function)
// returns. The done channel is buffered so the abandoned worker never blocks.
func (p *Program) runWorker(env map[string]any, ctx context.Context) (any, error) {
	done := make(chan runResult, 1) // buffered so an abandoned worker never blocks
	go func() {
		// If safeRun's body unwinds via runtime.Goexit (called from a custom
		// function), recover() returns nil and this deferred send still runs as
		// part of the goroutine's normal exit-defer chain — but res will carry
		// completed=false because the sentinel-set below never executed.
		var res runResult
		defer func() { done <- res }()
		v, err := p.safeRun(env)
		res = runResult{value: v, err: err, completed: true}
	}()

	if ctx == nil {
		res := <-done
		if !res.completed {
			return nil, &EvalError{
				Source: p.source,
				Cause:  fmt.Errorf("golem: evaluation aborted (custom function called runtime.Goexit or exited without returning)"),
			}
		}
		return res.value, res.err
	}

	select {
	case res := <-done:
		if !res.completed {
			return nil, &EvalError{
				Source: p.source,
				Cause:  fmt.Errorf("golem: evaluation aborted (custom function called runtime.Goexit or exited without returning)"),
			}
		}
		return res.value, res.err
	case <-ctx.Done():
		return nil, &TimeoutError{Source: p.source, Cause: ctx.Err()}
	}
}

// safeRun is the panic boundary. It defers a recover around expr.Run; a
// recovered error becomes EvalError{Cause: err}, any other recovered value
// becomes EvalError{Cause: fmt.Errorf("%v", r)}. No panic ever escapes.
func (p *Program) safeRun(env map[string]any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = &EvalError{Source: p.source, Cause: e}
			} else {
				err = &EvalError{Source: p.source, Cause: fmt.Errorf("%v", r)}
			}
			result = nil
		}
	}()

	out, runErr := expr.Run(p.program, env)
	if runErr != nil {
		return nil, classifyRunError(p.source, runErr)
	}
	return out, nil
}

// normalizeVars coerces numeric input variables to their declared schema type.
// This is golem's explicit, documented, numeric-only input coercion: an int
// supplied for a float64-declared slot is widened to float64 (and vice versa
// for integral floats). It is NOT silent magic across kinds — a string supplied
// for a numeric slot yields a TypeMismatchError. Variables with no schema entry
// are passed through unchanged.
func normalizeVars(env map[string]any, schema Vars) error {
	if schema == nil {
		return nil
	}
	for name, declared := range schema {
		v, ok := env[name]
		if !ok || v == nil {
			continue
		}
		coerced, err := coerceToDeclared(v, declared)
		if err != nil {
			return &TypeMismatchError{
				Expected: typeName(declared),
				Actual:   typeName(v),
				Detail:   fmt.Sprintf("variable %q", name),
			}
		}
		env[name] = coerced
	}
	return nil
}

// coerceToDeclared coerces v toward the kind of declared. Only numeric<->numeric
// coercion is performed; everything else must already match.
func coerceToDeclared(v, declared any) (any, error) {
	switch declared.(type) {
	case float64:
		switch n := v.(type) {
		case float64:
			return n, nil
		case int:
			return float64(n), nil
		case int64:
			return float64(n), nil
		case int32:
			return float64(n), nil
		case float32:
			return float64(n), nil
		}
		return nil, errTypeMismatch
	case int, int64:
		switch n := v.(type) {
		case int:
			return n, nil
		case int64:
			return n, nil
		case int32:
			return int(n), nil
		case float64:
			if n == float64(int64(n)) {
				return int(n), nil
			}
			return nil, errTypeMismatch
		}
		return nil, errTypeMismatch
	default:
		// Non-numeric declared type: pass through; expr's type-checker is the
		// authority for non-numeric mismatches.
		return v, nil
	}
}

var errTypeMismatch = fmt.Errorf("numeric coercion failed")
