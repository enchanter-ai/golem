// Command basic demonstrates idiomatic use of the golem expression engine:
// a declared schema (LOUD by default), compile-once / run-many with a reusable
// Program, booleans and ternaries, strings, host-registered functions plus the
// curated math stdlib, inline let, the strict/null policy, and concurrent
// evaluation of one shared Program from many goroutines.
//
// Run it with:
//
//	go run ./examples/basic
package main

import (
	"errors"
	"fmt"
	"sync"

	"github.com/enchanter-ai/golem"
)

func main() {
	specExample()
	boolAndStrings()
	customFunctionsAndMath()
	loudByDefault()
	lenientNullPolicy()
	concurrentReuse()
}

// specExample evaluates the take-home's flagship expression. The schema declares
// x as a float64; the int input 5 is normalized to the float64 slot per golem's
// documented numeric model, so the result is 14.
func specExample() {
	e := golem.New(golem.WithVariables(golem.Vars{"x": 0.0}))

	p, err := e.Compile("2 + 3 * (x - 1)")
	if err != nil {
		panic(err)
	}

	v, err := p.Eval(golem.Vars{"x": 5}) // int 5 -> float64 schema slot
	if err != nil {
		panic(err)
	}

	f, _ := v.AsFloat()
	fmt.Printf("2 + 3 * (x - 1) with x=5  => %v\n", f) // => 14
}

// boolAndStrings shows booleans, the ternary, string operations, and the
// null-coalescing operator — all provided natively by the underlying engine.
func boolAndStrings() {
	e := golem.New(golem.WithStrictVars(false)) // lenient: user?.email may be absent

	decision, err := e.Eval(
		`status == "active" and score > 0.8 ? "promote" : "hold"`,
		golem.Vars{"status": "active", "score": 0.91},
	)
	if err != nil {
		panic(err)
	}
	s, _ := decision.AsString()
	fmt.Printf("ternary decision         => %q\n", s) // => "promote"

	email, err := e.Eval(`user?.email ?? "no-email"`, golem.Vars{"user": nil})
	if err != nil {
		panic(err)
	}
	es, _ := email.AsString()
	fmt.Printf("null-coalesced email     => %q\n", es) // => "no-email"

	upper, err := e.Eval(`upper(trim(name)) + "!"`, golem.Vars{"name": "  ada  "})
	if err != nil {
		panic(err)
	}
	us, _ := upper.AsString()
	fmt.Printf("string ops               => %q\n", us) // => "ADA!"
}

// customFunctionsAndMath registers a host function by name and opts into the
// curated math.* allowlist; expr provides the implementations, golem only wires
// them in.
func customFunctionsAndMath() {
	clamp := func(x, lo, hi float64) float64 {
		switch {
		case x < lo:
			return lo
		case x > hi:
			return hi
		default:
			return x
		}
	}

	e := golem.New(
		golem.WithMathStdlib(),
		golem.WithFunction("clamp", clamp),
	)

	v, err := e.Eval("clamp(sqrt(area), 0, 100)", golem.Vars{"area": 2500.0})
	if err != nil {
		panic(err)
	}
	f, _ := v.AsFloat()
	fmt.Printf("clamp(sqrt(2500),0,100)  => %v\n", f) // => 50

	// Inline let binding (a VALUE binding, not a user-authored function body).
	let, err := e.Eval("let half = area / 2.0; half + 1", golem.Vars{"area": 10.0})
	if err != nil {
		panic(err)
	}
	lf, _ := let.AsFloat()
	fmt.Printf("let half = area/2; half+1=> %v\n", lf) // => 6
}

// loudByDefault shows the LOUD contract: a typo'd top-level variable is a
// COMPILE error carrying a "did you mean" suggestion, never a silent zero.
func loudByDefault() {
	e := golem.New(golem.WithVariables(golem.Vars{"revenue": 0.0}))

	_, err := e.Compile("revenu * 2") // typo: revenu
	var undef *golem.UndefinedVariableError
	if errors.As(err, &undef) {
		fmt.Printf("typo caught at compile    => %v\n", undef) // did you mean "revenue"?
	} else {
		panic(fmt.Sprintf("expected UndefinedVariableError, got %v", err))
	}
}

// lenientNullPolicy shows the opt-in lenient policy: an undeclared variable
// resolves to nil at runtime instead of failing to compile.
func lenientNullPolicy() {
	e := golem.New(golem.WithStrictVars(false))

	v, err := e.Eval("missing ?? 42", golem.Vars{})
	if err != nil {
		panic(err)
	}
	i, _ := v.AsInt()
	fmt.Printf("lenient missing ?? 42    => %v\n", i) // => 42
}

// concurrentReuse compiles one Program and evaluates it from many goroutines.
// expr's compiled *vm.Program is safe for concurrent reuse, so no external
// synchronization of the Program is required.
func concurrentReuse() {
	e := golem.New(golem.WithVariables(golem.Vars{"x": 0.0}))
	p, err := e.Compile("2 + 3 * (x - 1)")
	if err != nil {
		panic(err)
	}

	const goroutines = 64
	var wg sync.WaitGroup
	results := make([]float64, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			v, err := p.Eval(golem.Vars{"x": i})
			if err != nil {
				panic(err)
			}
			f, _ := v.AsFloat()
			results[i] = f
		}(i)
	}
	wg.Wait()

	fmt.Printf("concurrent x=10          => %v (one shared Program, %d goroutines)\n",
		results[10], goroutines) // x=10 => 29
}
