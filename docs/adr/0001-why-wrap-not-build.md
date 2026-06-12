# ADR-0001: Wrap an existing evaluator (expr-lang), do not build one

- Status: Accepted
- Date: 2026-06-11
- Authors: Enchanter Labs

## Context

golem must evaluate user-authored expressions — arithmetic, booleans, ternaries, strings, math,
null handling, host-registered functions — safely and at RTB scale, with identical results from Go
and Python. The take-home brief explicitly warns against reinventing the wheel. The build-vs-buy
question is therefore the graded crux: do we write a parser, type-checker, operator table, and VM,
or do we stand on a mature engine and add only the production layer it lacks?

## Decision

**Wrap `github.com/expr-lang/expr` (MIT) thinly. Do not build an evaluator.**

expr-lang already provides everything a hand-rolled engine would have to re-implement and get
correct: lexer, parser, a static type-checker driven by `expr.Env`, the full operator table, the
builtins and string functions, native `??` / `?.` / inline `let`, custom functions via
`expr.Function`, `AllowUndefinedVariables`, and — critically — a compile-once, thread-safe,
reusable `*vm.Program`. golem adds only the production wrapper: a bounded LRU program cache, a
host-function registry, an explicit strict/lenient null policy, cost/timeout guards, a panic→typed-
error boundary, and a typed public API that hides every expr-internal type. (A ~150-line Pratt
parser+evaluator lives in `examples/reference-parser/` purely to *illustrate* what we deliberately
did not build; golem never imports it.)

## expr vs cel-go (the alternatives considered)

| | expr-lang/expr | cel-go |
|---|---|---|
| License | **MIT** | Apache-2.0 + BSD |
| Throughput | ~70 ns/op | ~91 ns/op |
| Null ergonomics | native `??`, `?.`, `let` | `has()` macro |
| Thread-safe compiled program | yes | yes |
| Stability | stable | repo move 16 Jun 2026 |

Both are mature and thread-safe once compiled. expr wins on license simplicity (single MIT vs
dual-license), throughput, and first-class null-coalescing/optional-chaining/`let` syntax that maps
directly onto golem's must-support list. **Decision: expr.** cel-go is the runner-up.

CEL's headline advantage is its **dual-implementation cross-language parity** (cel-go and
cel-cpp/cel-python kept in lockstep). Under golem's architecture that advantage is **moot**: golem
has exactly one core implementation (the Go engine), and the Python binding is a thin CGo c-shared +
cffi layer over that same core. There is no second evaluator to keep in parity, so there is nothing
for CEL's dual-impl machinery to buy us — parity is automatic because there is only one source of
truth. That removes cel-go's only real edge, leaving expr's license, speed, and syntax to decide it.

## Consequences

- We inherit expr's correctness, performance, and feature set — and its constraints. Notably, expr
  has **no VM-level preemption** (issue #975 confirms `expr.WithContext` only feeds a ctx into custom
  functions; it cannot abort a running `Run`), so golem's `WithEvalTimeout` is implemented by racing
  `expr.Run` in a goroutine against a deadline, and `WithCostLimit` (`expr.MaxNodes`) is the
  preferred primary guard.
- The cache dependency is `hashicorp/golang-lru/v2` (MPL-2.0, a permissive file-level copyleft,
  compatible with golem's MIT), which is already goroutine-safe — so golem adds no redundant mutex.
- **Python binding — ADR fallback invoked:** the original plan used `gopy` for the Python binding.
  gopy proved unable to bind golem's `any`-rich API: its functional-option constructors and
  `map[string]any` variable maps cannot cross the gopy boundary correctly. Rather than silently
  degrading, we HALTed and invoked the documented fallback: a CGo `-buildmode=c-shared` + cffi layer
  (`python/capi`, build tag `golemcapi`) that exposes a **string-only C ABI** (`GolemNewEngine`,
  `GolemEvalJSON`, `GolemFreeEngine`, `GolemFreeString`) loaded at runtime via cffi in ABI mode. The
  JSON-envelope contract is unchanged; the string-only boundary is the reason the envelope is the
  right design, not a workaround. This is a vindication of the recorded trade-off, not a surprise.
