package golem_test

import (
	"fmt"

	"github.com/enchanter-ai/golem"
)

// example_test.go holds godoc-runnable examples. They are in package golem_test
// (black-box) so they document the PUBLIC API exactly as a consumer would use
// it, and their // Output: comments are verified by `go test`.

// ExampleEngine_Eval shows the spec expression: declare x as a float64, then
// evaluate 2 + 3 * (x - 1) with x supplied as an int (coerced to the schema).
func ExampleEngine_Eval() {
	e := golem.New(golem.WithVariables(golem.Vars{"x": 0.0}))
	v, err := e.Eval("2 + 3 * (x - 1)", golem.Vars{"x": 5})
	if err != nil {
		panic(err)
	}
	f, _ := v.AsFloat()
	fmt.Println(f)
	// Output: 14
}

// ExampleEngine_Compile shows compile-once / run-many: compile the program once
// and evaluate it repeatedly (each Eval is cheap and concurrency-safe).
func ExampleEngine_Compile() {
	e := golem.New(golem.WithVariables(golem.Vars{"x": 0.0}))
	p, _ := e.Compile("x * x")
	for _, x := range []float64{2, 3, 4} {
		f, _ := p.EvalFloat(golem.Vars{"x": x})
		fmt.Printf("%v ", f)
	}
	// Output: 4 9 16
}

// ExampleProgram_EvalString shows a bool + ternary + string result.
func ExampleProgram_EvalString() {
	e := golem.New(golem.WithVariables(golem.Vars{"status": "", "score": 0.0}))
	p, _ := e.Compile(`status == "active" and score > 0.8 ? "promote" : "hold"`)
	s, _ := p.EvalString(golem.Vars{"status": "active", "score": 0.9})
	fmt.Println(s)
	// Output: promote
}

// ExampleWithFunction shows a host-registered function plus the math stdlib.
func ExampleWithFunction() {
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
	v, _ := e.Eval("clamp(sqrt(area), 0.0, 100.0)", golem.Vars{"area": 25.0})
	f, _ := v.AsFloat()
	fmt.Println(f)
	// Output: 5
}

// ExampleWithStrictVars_loud demonstrates the LOUD contract: a typo in a
// declared variable name is a COMPILE error, not a silent zero.
func ExampleWithStrictVars_loud() {
	e := golem.New(golem.WithVariables(golem.Vars{"revenue": 0.0}))
	_, err := e.Compile("revenu * 2") // typo: revenu
	fmt.Println(err)
	// Output: golem: undefined variable "revenu" (did you mean "revenue"?)
}

// ExampleWithStrictVars_lenient shows opt-in lenient mode where an undeclared
// variable resolves to nil and ?? supplies a default.
func ExampleWithStrictVars_lenient() {
	e := golem.New(golem.WithStrictVars(false))
	v, _ := e.Eval(`missing ?? "default"`, nil)
	s, _ := v.AsString()
	fmt.Println(s)
	// Output: default
}

// ExampleEngine_EvalJSON shows the binding boundary: a JSON variables object in,
// a type-tagged JSON envelope out (no Go error crosses the boundary).
func ExampleEngine_EvalJSON() {
	e := golem.New(golem.WithVariables(golem.Vars{"x": 0.0}))
	fmt.Println(e.EvalJSON("2 + 3 * (x - 1)", `{"x":5}`))
	// Output: {"ok":true,"type":"float","value":14}
}
