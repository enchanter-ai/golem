# Contributing to golem

Thanks for your interest in `golem`. This document covers how to set up, build,
test, and propose changes. By contributing you agree your contributions are
licensed under the project's [MIT License](LICENSE).

## Guiding principles

golem is a **thin wrapper** around [`expr-lang/expr`](https://github.com/expr-lang/expr).
The single most important rule:

> **Do not re-implement what a mature dependency already provides.** If a change
> adds a parser, a type-checker, an operator table, a builtin, or a hand-rolled C
> ABI, it is almost certainly out of scope — expr (for evaluation) and gopy (for
> the binding) already do that work. golem adds only the production layer: bounded
> cache, typed errors, null policy, panic boundary, host-function ergonomics,
> cost/timeout guards, and a typed public API.

Other invariants:

- **No expr-internal type leaks** through the public API, error fields, or any
  exception message.
- **No panic escapes.** Recover at the boundary and return a typed error.
- **Honest numbers.** Benchmark and coverage claims report real measured values
  — never fabricated.
- **Scope guard (v1):** no Web3 / on-chain registry / Ed25519 / MCP server /
  model-tier logic / Explain Mode.

## Development setup

### Go core

Requires **Go 1.22+**.

```sh
go build ./...
go test ./...
go test -race ./...
go test -bench=. -benchmem ./...
go test -fuzz=FuzzCompile -fuzztime=30s
```

Linters and security gates (CI runs these on every PR):

```sh
golangci-lint run
staticcheck ./...
govulncheck ./...
```

### Python binding (tier-2)

The Python binding is generated with [`gopy`](https://github.com/go-python/gopy)
and requires a Go toolchain **and** a C toolchain to build locally:

```sh
make python   # runs `gopy build` into python/golem/
cd python && python -m pytest
```

gopy-generated files are **not** committed. Only `python/golem/__init__.py`,
`python/pyproject.toml`, and `python/tests/` are checked in.

## Pull requests

1. Open an issue first for anything larger than a bug fix or doc tweak — it saves
   everyone time if the design is agreed up front.
2. Branch from `main`. Keep the change surgical and focused on one concern.
3. Add or update tests. New behavior needs a table-driven test; edge cases need
   at least one case each. Concurrency-relevant changes need a `-race` test.
4. Run the full gate set above locally; all must be green.
5. Update `CHANGELOG.md` under `[Unreleased]`.
6. Keep the public API engine-agnostic — no new expr types in signatures,
   fields, or messages.

### Commit and PR style

- Conventional, present-tense commit subjects (`core: add WithCostLimit guard`).
- Reference the issue the PR closes.
- A green CI run (`ci.yml`) is required before review.

## Reporting bugs

Open an issue with a minimal reproducing expression, the declared variable
schema, the Go (or Python) version, and the observed vs. expected result. For
security-sensitive reports, follow [SECURITY.md](SECURITY.md) instead of the
public tracker.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating you are expected to uphold it.
