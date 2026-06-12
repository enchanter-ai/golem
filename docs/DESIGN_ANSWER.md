# Design answer — "it always evaluates to 0"

*Enchanter Labs*

## Problem

A data scientist reports that their expressions "always evaluate to 0, but it makes no
sense." This is the **silent-wrong** failure: the engine returns a value, so nothing looks
broken, but the value is meaningless. Four root causes produce it, all of them invisible at
the call site:

- **Integer-division truncation.** `5 / 2` is `2`, not `2.5`; `50 / 100` is `0`. When both
  operands are ints, the result floors — the single most common "always 0" trap.
- **Undefined / typo'd variable → nil/zero.** `revenu` for `revenue` resolves to nil, and
  arithmetic on nil collapses toward 0. Nested-key typos (`user.incom`) slip past top-level checks.
- **Implicit coercion.** A string or bool quietly coerced into a numeric context lands on 0.
- **Default-fill / nil-masking.** A defensive `user?.score ?? 0` returns the `0` default
  whenever any upstream link is missing — a genuine data gap becomes indistinguishable from a
  real zero.

The common thread: the engine has the information to know the result is suspect, but never
surfaces it. (This is a proposal — not built in v1.)

## Proposal: Explain Mode

One opt-in mode that makes the engine explain *why* a result is what it is, across three surfaces:

- **Compile-time lint.** Before the first run, flag the structural traps: undeclared/typo'd
  identifiers with a "did you mean `revenue`?" suggestion, integer-division sites at risk of
  truncation (`int / int` in a numeric context), and nil-masking `?.` / `??` chains whose
  default could hide a missing field. Authors see warnings while writing, not silent zeros in
  production.
- **Runtime trace.** Evaluate with instrumentation that returns each sub-expression's
  intermediate value, so the author sees exactly where the number became `0`/`nil` — e.g.
  `revenue (=0, int) / 100 => 0` pinpoints the int-division step.
- **Strict-by-default in the UI.** The authoring surface defaults to strict mode (typos and
  unresolved variables are loud, actionable errors, not silent zeros) and renders lint warnings
  inline, with a one-click "explain this result" that opens the trace.

## Why it helps

Each cause maps to a surface: lint catches truncation and typos before a run, the trace
localizes coercion and default-fill at the offending node, and strict-by-default converts the
silent zero into an error the user can act on. The diagnosis moves from "it makes no sense" to
"line 1, `revenue` is undefined" — without the author reading engine internals.
