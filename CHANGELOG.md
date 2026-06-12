# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-12

Initial release of `golem` — a thin, idiomatic, production-hardened wrapper
around [`expr-lang/expr`](https://github.com/expr-lang/expr).

### Added

- **Go core** — a thin wrapper over `expr-lang/expr` v1.17.x that adds only the
  production layer expr lacks:
  - `Engine` with a compile-once / run-many model and a bounded LRU program
    cache (`hashicorp/golang-lru/v2`, intrinsically goroutine-safe — no extra
    mutex).
  - Immutable, concurrency-safe `Program`; `Eval`, plus typed accessors
    `EvalBool` / `EvalFloat` / `EvalInt` / `EvalString`.
  - `Value` with `AsAny` / `AsBool` / `AsFloat` / `AsInt` / `AsString` typed
    accessors; no expr-internal type leaks through the public API.
  - Typed-error taxonomy: `ParseError`, `UndefinedVariableError` (with a "did you
    mean" suggestion), `TypeMismatchError`, `EvalError`, `CostLimitError`,
    `TimeoutError`.
  - Panic→error boundary (`safeRun`): a recovered panic becomes an `EvalError`,
    never an escaped panic.
  - Options: `WithVariables`, `WithFunction`, `WithMathStdlib`, `WithStrictVars`
    (default strict/LOUD), `WithCacheSize`, `WithEvalTimeout`, `WithCostLimit`.
  - Explicit numeric model: `AsFloat`/`EvalFloat` widen `int`→`float64`;
    `AsInt`/`EvalInt` reject true floats; Eval-input numeric normalization to the
    declared schema type.
- **Python binding (tier-2, best-effort)** — a CGo c-shared library (`libgolem`,
  compiled with `go build -buildmode=c-shared`) exposing a string-only C ABI
  (`GolemNewEngine`, `GolemEvalJSON`, `GolemFreeEngine`, `GolemFreeString`), loaded
  at runtime via **cffi in ABI mode** (no compile step at install). Dynamic var maps
  cross the boundary as a JSON envelope via `EvalJSON`; the Python wrapper restores
  the typed value and raises the mapped exception. Wheels built with `cibuildwheel`
  so `pip install enchanter-golem` needs no Go toolchain.
- **CI** — `ci.yml` (build / vet / test / `-race` / golangci-lint / staticcheck /
  govulncheck / coverage) and `wheels.yml` (c-shared + cibuildwheel matrix + a
  no-Go-toolchain wheel smoke test); `dependabot.yml`.
- **Docs** — README, write-up, design answer, ADR 0001 (why wrap not build), and
  an illustrative-only reference parser under `examples/`.

### Known limitations (v1)

- Custom Go functions run with full Go permissions; sandboxing beyond the panic
  boundary is a v2 roadmap item.
- Nested map-key typos (`obj.typo`) silently return `nil` — only top-level
  variable typos are caught at compile time with a map env.
- The Python binding does not accept Python callables as custom functions and
  does not return collection results (both v2 roadmap).

[Unreleased]: https://github.com/enchanter-ai/golem/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/enchanter-ai/golem/releases/tag/v0.1.0
