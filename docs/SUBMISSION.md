# golem — Arpeely Submission

*Enchanter Labs · v0.1.0 · MIT*

## Summary

**golem** is a production-grade expression-evaluation library written in **Go**, plus a
**Python binding** as a bonus. It evaluates author-written expressions such as
`2 + 3 * (x - 1)` or `status == "active" and score > 0.8 ? "promote" : "hold"` against a set
of named variables, returning a typed result or one of seven typed errors — never a silent
zero. The Go core is a thin, idiomatic wrapper over `github.com/expr-lang/expr` (MIT): it
compiles each expression once to an immutable program, caches that program in a bounded LRU,
and evaluates it concurrently from many goroutines on the hot path. A strict "loud" variable
schema turns typo'd or undeclared identifiers into compile-time errors instead of silent
zeros, and a panic→typed-error boundary guarantees no engine panic escapes the public API.
The Python binding (`pip install enchanter-golem`, `import golem`) loads the same Go core as a
CGo `c-shared` library (`libgolem`) over a string-only JSON ABI via cffi — proving the
single-core / many-bindings parity thesis.

## Requirements checklist

Each requirement from the take-home brief, mapped to where it is satisfied (source and/or test).

| # | Requirement | Where satisfied (source) | Verified by (test) |
|---|-------------|--------------------------|--------------------|
| 1 | Evaluate an expression against named variables (`2 + 3 * (x - 1)`) | `engine.go` — `Engine.Eval(src, vars)` | `TestSpecFlagshipExample`, `TestArithmetic` |
| 2 | `+ - * /`, parentheses, variable lookup + common math | `engine.go`; math allowlist | `TestArithmetic`, `TestModulo`, `TestMathBuiltins`, `TestMathStdlib` |
| 3 | Booleans + conditional ops: ternary, `and`, `or` | engine operator table (expr) | `TestBooleansAndTernary`, `TestLogicalOperators`, `TestBooleanShortCircuit` |
| 4 | Strings | engine string support (expr) | `TestStrings`, `TestStringPredicates`, `TestSplit`, `TestUnicodeStrings` |
| 5 | Define / use variables (external map + inline `let`) | `Vars` map; inline `let` | `TestSpecFlagshipExample`, `TestInlineLet` |
| 6 | Define / use functions (host-registered, called by name) | `WithFunction(...)` option | `TestCustomFunction`, `TestCustomFunctionNumericCoercion`, `ExampleWithFunction` |
| 7 | Runs millions/sec for similar expressions (→ compile-once + cache) | bounded LRU program cache (`golang-lru/v2`) | `TestCompileIsCached`, `TestEvalPopulatesCache`, `TestCacheBoundedEviction`, `BenchmarkEvalCached`, `BenchmarkCompileVsCached` |
| 8 | Thread-safe | immutable `*Program`, goroutine-safe cache | `TestConcurrentEvalSharedProgram`, `TestConcurrentEngineEval`, `TestConcurrentCustomFunction`, `BenchmarkEvalConcurrent` (`go test -race`) |
| 9 | Null / undefined handled throughout | strict-by-default schema; `??` / `?.` | `TestStrictUndefinedVariableIsCompileError`, `TestLenientUndefinedVariableIsNil`, `TestNullCoalesceAndOptionalChain`, `TestNilOperandArithmetic`, `TestNestedKeyTypoIsSilentNil` |
| 10 | Open-source + pulled in as a dependency | MIT, `go get github.com/enchanter-ai/golem`; `pip install enchanter-golem` | importable package; `ExampleEngine_Eval` |

Supporting hardening beyond the brief: typed errors and panic boundary
(`TestCoreFix_*`, `TestNoExprTypeLeaksInErrors`), cost/timeout guards (`TestCostLimit`,
`TestRegressionTimeoutNoHungCaller`), and a fuzz target for escaped panics (`FuzzCompile`).

## Submission artifacts

| Artifact | Location |
|----------|----------|
| Working code (repository) | <https://github.com/enchanter-ai/golem> |
| Write-up (≤ 1 page: approach, author-vs-AI, vague-by-design decisions) | [docs/WRITEUP.md](./WRITEUP.md) |
| Design answer ("expressions always evaluate to 0", ≤ ½ page) | [docs/DESIGN_ANSWER.md](./DESIGN_ANSWER.md) |
| AI chat transcript | [docs/ai-transcript.md](./ai-transcript.md) |

The library description is the repository README.

## How to run / verify

**Go core (the submitted library):**

```sh
git clone https://github.com/enchanter-ai/golem
cd golem
go test ./...            # full suite
go test -race ./...      # thread-safety under the race detector
go test -bench=. ./...   # cached-vs-cold ns/op
```

**Python binding (bonus):**

```sh
pip install enchanter-golem
python -c "import golem; print(golem.evaluate('2 + 3 * (x - 1)', {'x': 4}))"
```

## Language note

The brief allows **Python OR Go**; the chosen submission language is **Go**. The Go core
satisfies every requirement above. The **Python binding is a bonus** — it binds the same
compiled Go core through a CGo `c-shared` library and demonstrates the one-core / many-bindings
architecture, but the evaluated production surface is the Go library.
