# golem — Write-up

*Enchanter Labs*

`golem` is an expression evaluator that **compiles once and runs millions of times per second
with no silent failures**. A backend hands it an expression authored by a non-engineer —
`2 + 3 * (x - 1)`, `status == "active" and score > 0.8 ? "promote" : "hold"` — plus named
variables, and gets back a typed result or a typed error.

## Approach

The brief said "don't reinvent the wheel," so golem reinvents nothing it can stand on. It is a
**thin, idiomatic wrapper around `github.com/expr-lang/expr` (MIT)** — the fastest,
feature-complete, thread-safe Go expression engine. expr already ships the lexer, parser,
type-checker, operator table, math builtins, `??` / `?.` / inline `let`, host functions, and a
reusable compiled program. Reaching for it instead of hand-rolling a parser/VM is the central
judgment call.

Three "by default"s drive the rest:

- **Fast** — `expr.Compile` produces a thread-safe `*vm.Program`; golem caches it in a **bounded
  LRU** and reuses it, so steady state is run-only (~0.8–1 µs per cached eval, hundreds of ns
  under parallelism — README benchmark). Compile-once + cache is what turns the "millions/sec for
  similar expressions" requirement into a measured fact.
- **Safe** — a curated math allowlist, an AST-cost budget, a wall-clock timeout, and a
  panic→typed-error boundary at every eval. No engine panic escapes the public API; one immutable
  Program is shared safely across goroutines.
- **Loud** — a declared variable schema turns a typo'd or undeclared top-level identifier into a
  *compile-time* `UndefinedVariableError` (with a "did you mean" suggestion), **not a silent
  zero**. This directly answers the bonus "expressions always evaluate to 0" design question: the
  cure is to make undeclared names loud at compile time rather than resolve them to a zero value.

A Go core was chosen because the best-in-class evaluator being wrapped *is* Go (`expr-lang`), and
Go meets the millions/sec + thread-safe bar natively. A CGo `c-shared` library (`libgolem`,
loaded via cffi, `pip install enchanter-golem`) binds Python to that same core — the product goal
— so every binding inherits core features with zero feature drift.

## What I decided vs. what the AI decided

**The author (Enchanter Labs) made the architecture and judgment calls.** Wrap-not-build; Go core
for performance and thread-safety; LOUD-by-default null policy; the Python-first product goal; and
the full public API shape (the `Engine`/`Program`/`Value` surface, the `Option` set, the typed-error
taxonomy, the numeric coercion / int→float widening model). The author also **caught drift and
steered**: the original plan used `gopy` for the binding, but gopy can't bind golem's `any`-rich
API cleanly, so the author directed the **pivot to a string-only JSON C ABI loaded via cffi** —
the reason the language boundary is JSON by design, not a workaround.

**The AI — robit, enchanter-ai's agent runtime (on Claude Opus) — executed against that spec.** It generated the Go source, the table-driven /
fuzz / concurrency / benchmark tests, and the CI + wheel-packaging workflows; ran the **expr-lang
vs. cel-go** research (ADR-0001 records expr winning on speed, native `??`/`?.`/`let`, and a single
permissive license); and ran a production-grade hardening loop. **The author verified correctness**
on a clean checkout — `go build` / `vet` / `test` / `test -race` with many goroutines on one
Program, the cached-vs-cold benchmark, the `FuzzCompile` panic target, the Go↔Python parity
corpus — and adversarially fact-checked the load-bearing claims (expr has no VM-level preemption;
golang-lru/v2 is MPL-2.0 and already thread-safe) against primary sources before accepting code.

## Vague-by-design rulings

The brief left two things underspecified; golem rules on each and documents why:

- **"Define functions inside expressions"** means **host-registered Go functions called by name**
  (register `clamp`, call `clamp(x, 0, 1)`) — *not* user-authored function bodies in expression
  text. User-authored bodies would break the safety, cache, and termination guarantees, so they
  are deliberately out. Inline `let` value-binding *is* supported.
- **Null/undefined** is **strict (LOUD) by default**: an undeclared top-level identifier is a
  compile error, with `??` / `?.` available for explicit null handling. An opt-in lenient mode
  (`WithStrictVars(false)`) resolves undeclared vars to nil. Documented caveat: with a map env,
  unknown *nested* keys still return nil silently — only top-level typos are caught at compile
  time. Go's `5/2 == 2` integer division, the #1 silent-zero trap, is called out explicitly.
