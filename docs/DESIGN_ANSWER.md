# Design answer — "it always evaluates to 0"

*Enchanter Labs*

A PM reports that some users' expressions "always evaluate to 0." This is the classic
**silent-wrong** failure: the engine returns a value, so nothing looks broken, but the value is
meaningless. Below are the most common causes and the feature that surfaces them. (This is a design
proposal — not built in v1.)

## Top 3 causes

1. **Integer division truncation.** `5 / 2` is `2`, not `2.5`, because both operands are integers
   (Go semantics, inherited from the engine). `revenue / 100` where `revenue` is an int silently
   floors. This is the single most common "always 0" trap: `50 / 100 == 0`.
2. **Undefined / typo'd variable resolving to nil/zero.** In lenient mode an unknown identifier
   (`revenu` for `revenue`) resolves to nil, and arithmetic on nil collapses toward 0. With a map
   env, an unknown *nested* key (`user.incom`) also returns nil silently — only top-level typos are
   caught at compile time.
3. **Nil-masking via `??` / `?.`.** A defensive chain like `user?.score ?? 0` does exactly what it
   says: when *any* link is missing it returns the `0` default — so a genuine data gap is
   indistinguishable from a real zero, and the result is "always 0" whenever the upstream field is
   absent. (Adjacent contributors: implicit numeric coercion, operator precedence, and
   empty-aggregate defaults.)

## Feature: Explain Mode

A single opt-in mode that makes the engine explain *why* a result is what it is, on three surfaces:

- **Compile-time lint.** Before the first run, flag the structural traps: unknown identifiers with a
  "did you mean `revenue`?" suggestion, integer-division sites at risk of truncation
  (`int / int` in a numeric context), and nil-masking chains (`?.` / `??` whose default could hide a
  missing field). Authors see these as warnings while writing, not as silent zeros in production.
- **Runtime trace.** Evaluate with instrumentation that returns the AST annotated with each
  sub-expression's value, so the author sees exactly where the number became `0`/`nil` — e.g.
  `revenue (=0, int) / 100 (=int) => 0` pinpoints the int-division step.
- **Strict-default UI surface.** The authoring UI defaults to strict mode (typos = compile errors)
  and surfaces the lint warnings inline next to the expression, with a one-click "explain this
  result" that renders the runtime trace. Strict-by-default is the front-line defense; the trace is
  the diagnostic when a result is still surprising.
