# golem — AI Chat Transcript (curated)

*Enchanter Labs*

## Tool used

The AI collaborator was **robit** — enchanter-ai's agentic runtime: an enforcement-first,
MCP-aware agent (a 7-phase lifecycle that injects conduct, vetoes unsafe actions, and masks
secrets) built on the Claude Agent SDK and running the **Claude Opus** model. It read and wrote
files in the repo directly, ran the Go toolchain and the test suite, and iterated on CI in-loop
rather than just emitting code blocks to copy out.

This file is a **curated, readable record** of the human↔AI collaboration, not the raw log.
The complete, unedited transcript lives in robit's local session log and is **exportable from its
session-transcript directory** (the per-session `.jsonl` under the user's
`~/.claude/projects/…` tree). What follows is the narrative of the load-bearing decisions — who
decided what, and why — distilled to roughly one page.

## Division of labor (the through-line)

- **Human** — judgment, architecture, and steering: the build-vs-buy ruling, the public API
  shape, the "loud-by-default" stance, the Python-first product goal, and every locked decision.
- **AI** — execution: deep research, code, table-driven / fuzz / concurrency / benchmark tests,
  CI and wheel packaging, and prose docs — all against the human's spec, with the human
  verifying correctness on a clean checkout.

## Key beats

**1. PDF → plan + deep research.** The human handed over the take-home brief (evaluate
expressions like `2 + 3 * (x - 1)` against named variables; arithmetic, booleans, ternaries,
strings, defined variables/functions, thread-safe, null-safe, millions/sec, open-source). The AI
turned it into a plan and ran a focused comparison of the two credible Go engines —
`expr-lang/expr` vs `google/cel-go`. Finding: **expr** wins on a single permissive **MIT**
license, higher throughput, and first-class `??` / `?.` / inline `let`; cel-go's headline edge
(dual-implementation cross-language parity) is moot under a single-core architecture. Recorded in
`docs/adr/0001-why-wrap-not-build.md`.

**2. Scaffolding the Go core (human sets architecture, AI builds).** The human fixed the
architecture up front: **wrap, do not build**; one **Go core** for the millions/sec + thread-safe
hot path; **LOUD by default** so a typo'd variable is a compile-time error, never a silent `0`.
The AI scaffolded that core as a thin wrapper over `expr-lang` — compile-once into an immutable
`*vm.Program`, a bounded `golang-lru/v2` cache, a 7-type error API, a strict `expr.Env` schema, a
`safeRun` panic→error boundary, plus cost-budget and timeout guards.

**3. Production-grade hardening loop (the real lesson).** An early pass scored well on a prose
quality rubric and *looked* done. The human pushed for **production-grade, not deploy-score**, and
directed independent **adversarial reviews + technical fact-checks**. Those caught
**build-breaking errors and overclaims the prose score had missed** — e.g. that `expr` has no
VM-level preemption (so a context can't abort a running `Run`, which forced the goroutine-raced
timeout design and made `WithCostLimit` the primary guard), and the exact license/thread-safety
facts of the LRU dependency. **Lesson banked: a passing quality score is not the same as
production-grade** — only an independent adversarial check against primary sources is.

**4. Python-first product goal.** The human reframed the deliverable around **non-technical
users**: a data scientist should `pip install enchanter-golem`, `import golem`, and define a
formula in a notebook — never touching a Go toolchain or seeing a goroutine, while a Go service
embeds the *same* core for identical answers (no parity drift).

**5. The gopy → cffi pivot.** The original binding plan was **gopy**. In practice gopy **could not
bind golem's `any`-rich API** — its functional-option constructors and `map[string]any` variable
maps don't cross the gopy boundary correctly. Rather than silently degrade, the AI **HALTed and
invoked the pre-planned fallback**: a CGo `-buildmode=c-shared` library (`libgolem`) exposing a
**string-only C ABI** (`GolemNewEngine`, `GolemEvalJSON`, `GolemFreeEngine`, `GolemFreeString`),
loaded at runtime via **cffi** in ABI mode. Dynamic data crosses as a type-tagged **JSON
envelope** — which is why the string-only boundary is the right design, not a workaround.

**6. CI to green wheels.** The AI iterated the GitHub Actions workflows (`ci.yml`, `wheels.yml`)
until they produced **prebuilt wheels across Linux, macOS, and Windows**, with the compiled Go
engine baked in, alongside `go build` / `go vet` / `go test` / `go test -race` and the Go↔Python
parity corpus.

## Verification

The human verified correctness independently on a **clean checkout** — running the acceptance
gates (`go build`, `go vet`, `go test`, `go test -race` with many goroutines sharing one
`Program`, the benchmark for real cached-vs-cold `ns/op`, the `FuzzCompile` target for escaped
panics, and the Go↔Python parity corpus) — and adversarially fact-checked the load-bearing
framework claims against primary sources before accepting the AI-generated code.
