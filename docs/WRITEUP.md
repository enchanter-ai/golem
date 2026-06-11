# golem — Write-up

*Enchanter Labs*

## What it is

An expression evaluator that **compiles once and runs millions of times per second with no
silent failures**. A backend service hands `golem` an expression authored by a non-engineer —
`2 + 3 * (x - 1)`, `status == "active" and score > 0.8 ? "promote" : "hold"` — and a set of
named variables, and gets back a typed result or a typed error. It is built for the RTB hot
path: compile an expression to an immutable program once, cache it, then evaluate that program
concurrently from many goroutines on every request.

Three "by default"s drive the design:

- **Fast** — `expr.Compile` produces a thread-safe `*vm.Program`; golem caches it in a bounded
  LRU and reuses it, so the steady state is run-only (~0.8–1 µs per cached eval, hundreds of ns
  under parallelism, measured — see the README benchmark table).
- **Safe** — a curated math allowlist, an AST-node cost budget, a wall-clock timeout, and a
  panic→typed-error boundary at every evaluation. No engine panic ever escapes the public API.
- **Loud** — a declared variable schema turns a typo'd or undeclared top-level identifier into a
  *compile-time* `UndefinedVariableError` (with a "did you mean" suggestion), not a silent zero.

## Approach

The brief said "don't reinvent the wheel," so golem reinvents nothing it can stand on. It is a
**thin, idiomatic wrapper around `github.com/expr-lang/expr` (MIT)** — the fastest, feature-complete,
thread-safe Go expression engine. expr already provides the lexer, parser, type-checker
(`expr.Env`), operator table, builtins, `??` / `?.` / inline `let`, custom functions
(`expr.Function`), and the reusable compiled program. golem adds **only** what expr lacks for a
production runtime: a bounded LRU program cache, a host-function registry with ergonomic numeric
coercion, an explicit strict/lenient null policy, a panic boundary, cost/timeout guards, and a
typed, engine-agnostic public API that never leaks an expr-internal type. That is the whole wrapper.

## Framework choices

- **Engine:** `expr-lang/expr` v1.17.x (MIT). Faster than cel-go, native `??`/`?.`/`let`, and a
  single permissive license. (ADR-0001 records why expr over cel-go.)
- **Cache:** `hashicorp/golang-lru/v2` (MPL-2.0, pinned v2.0.7) — already goroutine-safe, so golem
  wraps it directly with no redundant mutex rather than hand-rolling an LRU.
- **Stdlib only** for the panic boundary, numeric model, and JSON envelope.

## The CEL-expr-python precedent

The architecture — one compiled Go core, with other languages binding to it for automatic
cross-language parity and zero feature drift — is exactly the pattern Google shipped with
**CEL-expr-python (Mar 2026)**, which wraps a compiled CEL core in a thin Python skin. golem
applies the same thesis: one evaluator, one source of truth, every binding inherits core features
for free. (Why Go and not Rust/PyO3? Because the best-in-class evaluator we are binding *is* Go —
`expr-lang` — so the core language is chosen by the dependency, not by preference.)

## Vague-by-design rulings

The brief left three things underspecified; golem rules on each and documents it:

- **"Define functions inside expressions"** means **host-registered Go functions called by name**
  (register `clamp`, call `clamp(x, 0, 1)`), *not* user-authored function bodies in expression
  text. Inline `let` value-binding *is* supported.
- **"Define variables"** means both external (the `Vars` map) and inline (`let`).
- **Null/undefined** is **strict by default**: an undeclared top-level identifier is a compile
  error. `??`/`?.` are available; opt-in lenient mode (`WithStrictVars(false)`) resolves undeclared
  vars to nil. Documented caveat: with a map env, unknown *nested* keys still return nil silently —
  only top-level typos are caught at compile time. The #1 silent-zero trap, Go's `5/2 == 2` integer
  division, is called out explicitly.

## Python binding (tier-2 bonus)

A `gopy`-generated Python binding over the same Go core is included as a **demonstrated tier-2 bonus**
— it proves the parity thesis and gives `pip install enchanter-golem`, but the evaluated production surface is
the Go core, and the binding pays a gopy + JSON cost per call.

## Author vs AI

**The author (Enchanter Labs) specified** the architecture (single Go core wrapping expr-lang, with
a gopy-generated Python binding over a JSON-string boundary), the entire public API shape (the
`Engine`/`Program`/`Value` surface, the `Option` set, the typed-error taxonomy, the numeric
input-coercion and int→float widening model), and every locked decision (expr over cel-go,
golang-lru/v2 with no extra mutex, cost-limit as the primary guard with a goroutine-raced timeout as
secondary, the strict-by-default null policy, the JSON-envelope binding contract that avoids gopy
issue #254). **Fable 5 generated** the implementation against that spec: the Go source, the
table-driven / fuzz / concurrency / benchmark tests, the CI and wheel-packaging workflows, and the
prose docs. **The author verified correctness** by running the acceptance gates on a clean checkout
— `go build`, `go vet`, `go test`, `go test -race` with many goroutines sharing one Program, the
benchmark for real cached-vs-cold `ns/op`, the `FuzzCompile` fuzz target for escaped panics, and the
Go↔Python parity corpus — and by adversarially fact-checking the load-bearing framework claims (that
expr has no VM-level preemption, that golang-lru/v2 is MPL-2.0 and already thread-safe, that gopy's
`(string, error)` mapping is buggy) against primary sources before accepting the generated code.
