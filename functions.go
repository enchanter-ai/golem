package golem

import (
	"fmt"
	"math"
	"reflect"

	"github.com/expr-lang/expr"
)

// mathStdlib is the curated math.* allowlist exposed when WithMathStdlib is set.
// expr already provides abs/ceil/floor/round/min/max/sum as builtins; golem adds
// ONLY the math.* functions expr lacks (sqrt, pow, exp, log, log2, log10, the
// trig family, etc.), thinly delegating each to the standard library. golem does
// not re-implement any of these — they are stdlib math calls.
var mathStdlib = map[string]any{
	"sqrt":  math.Sqrt,
	"cbrt":  math.Cbrt,
	"pow":   math.Pow,
	"exp":   math.Exp,
	"log":   math.Log,
	"log2":  math.Log2,
	"log10": math.Log10,
	"sin":   math.Sin,
	"cos":   math.Cos,
	"tan":   math.Tan,
	"asin":  math.Asin,
	"acos":  math.Acos,
	"atan":  math.Atan,
	"atan2": math.Atan2,
	"hypot": math.Hypot,
	"trunc": math.Trunc,
	"mod":   math.Mod,
}

// functionOptions builds the expr.Option set that registers every
// host-provided custom function. Each Go func is wrapped THINLY: it is adapted
// to expr's func(params ...any) (any, error) shape via reflection, and the
// original typed func value is passed as a type witness so expr's checker still
// type-checks call sites. The same options must be applied at BOTH compile and
// run time — expr requires the function set to be present in both passes — so
// callers use this single builder for both.
func functionOptions(cfg *config) ([]expr.Option, error) {
	opts := make([]expr.Option, 0, len(cfg.functions)+len(mathStdlib))

	// Curated math.* stdlib first, so an identically-named user function (below)
	// overrides it in expr's later-wins option ordering.
	if cfg.mathStdlib {
		for name, fn := range mathStdlib {
			if _, shadowed := cfg.functions[name]; shadowed {
				continue
			}
			opt, err := functionOption(name, fn)
			if err != nil {
				return nil, err
			}
			opts = append(opts, opt)
		}
	}

	for name, cf := range cfg.functions {
		opt, err := functionOption(name, cf.fn)
		if err != nil {
			return nil, err
		}
		opts = append(opts, opt)
	}
	return opts, nil
}

// functionOption adapts a single Go func to an expr.Function option.
func functionOption(name string, fn any) (expr.Option, error) {
	rv := reflect.ValueOf(fn)
	if !rv.IsValid() || rv.Kind() != reflect.Func {
		return nil, fmt.Errorf("golem: WithFunction(%q): value is not a function (got %T)", name, fn)
	}
	rt := rv.Type()
	if rt.IsVariadic() {
		// Variadic adaptation via reflect.Call with a spread slice is possible
		// but the call-time arity handling is subtle; v1 supports fixed-arity
		// host functions only.
		return nil, fmt.Errorf("golem: WithFunction(%q): variadic functions are not supported in v1", name)
	}
	numIn := rt.NumIn()
	numOut := rt.NumOut()
	if numOut < 1 || numOut > 2 {
		return nil, fmt.Errorf("golem: WithFunction(%q): function must return (T) or (T, error), got %d return values", name, numOut)
	}
	errReturn := numOut == 2
	if errReturn && !rt.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil, fmt.Errorf("golem: WithFunction(%q): second return value must be error", name)
	}

	adapter := func(params ...any) (any, error) {
		if len(params) != numIn {
			return nil, fmt.Errorf("golem: function %q expects %d argument(s), got %d", name, numIn, len(params))
		}
		in := make([]reflect.Value, numIn)
		for i := 0; i < numIn; i++ {
			argType := rt.In(i)
			cv, err := convertArg(params[i], argType)
			if err != nil {
				return nil, fmt.Errorf("golem: function %q argument %d: %w", name, i+1, err)
			}
			in[i] = cv
		}
		out := rv.Call(in)
		if errReturn {
			if e, _ := out[1].Interface().(error); e != nil {
				return nil, e
			}
		}
		return out[0].Interface(), nil
	}

	// Pass the original typed func as a type witness so expr's checker still
	// type-checks call sites against the real signature.
	return expr.Function(name, adapter, fn), nil
}

// convertArg coerces a value coming from the expr VM into the Go type the host
// function declares. It handles the common numeric widening/narrowing (int <->
// float64) so a function declaring float64 parameters can be called with
// integer literals, mirroring golem's input numeric model.
func convertArg(v any, target reflect.Type) (reflect.Value, error) {
	if v == nil {
		return reflect.Zero(target), nil
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(target) {
		return rv, nil
	}
	if rv.Type().ConvertibleTo(target) && isNumericKind(rv.Kind()) && isNumericKind(target.Kind()) {
		return rv.Convert(target), nil
	}
	return reflect.Value{}, &TypeMismatchError{
		Expected: target.String(),
		Actual:   typeName(v),
	}
}

func isNumericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
