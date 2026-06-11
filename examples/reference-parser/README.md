# reference-parser — illustrative only

**This is not part of golem.** It is a self-contained ~150-line Pratt (top-down
operator-precedence) parser and tree-walking evaluator for arithmetic and boolean
expressions, included **purely to illustrate the layer golem deliberately does NOT
build**.

golem is a thin wrapper around [`expr-lang/expr`](https://github.com/expr-lang/expr),
which provides the lexer, parser, operator precedence, type-checker, and VM. This
example shows what writing that layer by hand looks like — exactly the wheel the
"don't reinvent the wheel" mandate tells us not to reinvent.

Properties:

- `package main` — a standalone command, **never imported by the `golem` package**.
- Not in the public API, not tested as part of golem, not on the production path.
- A single `float64` value type (booleans encoded as `1.0`/`0.0`) to stay small;
  the real engine handles strings, nulls, `??`/`?.`, `let`, custom functions, and a
  full type system — none of which are reimplemented here.

Run it:

```sh
go run ./examples/reference-parser
```

For real usage, see [`examples/basic`](../basic) (Go) and
[`examples/python`](../python) (the Python binding), which use the actual `golem`
package.
