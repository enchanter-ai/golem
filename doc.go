// Package golem is a thin, idiomatic wrapper around github.com/expr-lang/expr
// that turns it into a production expression-evaluation engine: compile-once /
// run-many with a bounded LRU program cache, a typed-error public API, an
// explicit null/strict policy, a panic boundary, and host-registered functions.
//
// golem deliberately does NOT re-implement parsing, type-checking, operators,
// builtins, or the VM — expr-lang provides all of those. No expr-internal type
// is ever exposed through golem's public API or its error messages.
//
// A Python binding generated from this same package by gopy lives under python/.
// It binds the Go core directly — there is no second evaluator and no parity
// drift — and crosses dynamic data as a JSON envelope produced by EvalJSON and
// NewEngineJSON.
package golem
