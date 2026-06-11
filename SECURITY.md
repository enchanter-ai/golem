# Security Policy

## Supported versions

golem is pre-1.0. Security fixes are applied to the latest released minor
version. Until 1.0, only the most recent `0.x` release line receives fixes.

| Version | Supported |
|---------|-----------|
| 0.1.x   | yes       |
| < 0.1   | no        |

## Reporting a vulnerability

**Do not open a public issue for a security vulnerability.**

Please report privately via GitHub's
[Private Vulnerability Reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability)
on the `enchanter-ai/golem` repository, or email **security@enchanterlabs.dev**.

Include:

- a description of the issue and its impact,
- a minimal reproducing expression, the declared variable schema, and any custom
  functions involved,
- the golem version and the Go (or Python) version,
- any suggested remediation.

We aim to acknowledge a report within **3 business days** and to provide a
remediation timeline within **10 business days**. We will coordinate disclosure
with you and credit you in the advisory unless you prefer to remain anonymous.

## Scope and the trust model

golem's security posture follows directly from its design — please calibrate
reports against it:

- **Evaluation is sandboxed at the language level.** golem evaluates only
  expressions over a declared variable schema and a host-provided function
  allowlist. It is **not** Turing-complete and exposes no I/O, filesystem,
  network, or process primitives by itself.
- **Custom Go functions are NOT sandboxed in v1.** A function you register with
  `WithFunction` runs with full Go permissions — the panic boundary catches
  panics, but it does not confine side effects. **Only register functions you
  trust** with inputs you control. A capability-confined custom-function sandbox
  is a v2 roadmap item. (Registering arbitrary *untrusted* Go functions is
  outside the v1 threat model.)
- **Resource exhaustion is bounded, not eliminated.** `WithCostLimit` (the
  preferred guard) bounds AST node visits; `WithEvalTimeout` bounds caller wait
  time. Because the underlying engine has no VM-level preemption, a pathological
  custom function can still occupy a goroutine past the deadline — see the README
  timeout section. Reports of unbounded growth in the **program cache** (it is a
  bounded LRU) or of a **panic escaping** the boundary are in scope.
- **No silent failure is a security property.** A report showing that a typo'd or
  type-mismatched expression produces a wrong value instead of a typed error (in
  strict mode) is in scope — golem's contract is to be loud.

## Dependencies

golem runs `govulncheck` in CI. Known-vulnerable dependency advisories are
tracked and patched via Dependabot. The evaluation engine is
[`expr-lang/expr`](https://github.com/expr-lang/expr) (MIT) and the program cache
is [`hashicorp/golang-lru/v2`](https://github.com/hashicorp/golang-lru)
(MPL-2.0).
