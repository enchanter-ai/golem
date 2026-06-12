# enchanter-golem

Fast, safe, **loud** expression evaluation for Python — a thin Pythonic skin over a
compiled Go core ([`expr-lang/expr`](https://github.com/expr-lang/expr)). Evaluate
expressions like `2 + 3 * (x - 1)` against named variables; a typo'd variable is a
**compile error**, not a silent `0`.

```python
import golem

e = golem.Engine(variables={"x": 0.0})
print(e.eval("2 + 3 * (x - 1)", {"x": 5}))   # => 14
```

## Install

```sh
pip install enchanter-golem        # the import is `import golem`
```

Prebuilt wheels embed the compiled Go core, so there is **no Go toolchain and no C
compiler** to install. Evaluation runs in Go; Python is just the interface.

## Why

- **Loud** — an undeclared or typo'd variable fails at compile time with a "did you
  mean" hint, instead of silently becoming `0`.
- **Safe** — a sandboxed function/math allowlist, cost + timeout guards, and no silent
  `NaN`/`Inf`.
- **One source of truth** — the same Go core powers the Go API, so Python and Go agree.

Full documentation, the Go API, and the architecture diagrams live in the project
README: <https://github.com/enchanter-ai/golem>

MIT © Enchanter Labs.
