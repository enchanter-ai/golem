# golem — AI Collaboration Transcript & Build Lifecycle

*Enchanter Labs*

This document is the **AI-transcript deliverable** for the take-home, expanded into a record of how
golem was built. It declares the **enchanter-ai lifecycle** — the internal agent platform and the
product/discipline applied at each phase — alongside the human↔AI division of labor. It is a
**curated, readable record**, not the raw log; the complete, unedited transcript lives in robit's
local session store (the per-session `.jsonl` under `~/.claude/projects/…`, exportable on request)
and is the ground truth behind everything summarized here.

---

## The runtime — robit

The AI collaborator was **robit**, enchanter-ai's agentic runtime: an enforcement-first, MCP-aware
agent built on the Claude Agent SDK and running **Claude Opus**. robit is not a chat window — it is
a **7-phase execution lifecycle** (intake → conduct injection → planning → action with engine
vetoes → verification → secret masking → learning) that reads and writes the repo directly, runs
the Go and Python toolchains, drives CI in-loop, and **hosts the rest of the enchanter-ai product
suite**, enabling each product by purpose as the work demands it. Every other product named below
ran *inside* robit's lifecycle and *under* vis's behavioral contract.

---

## The enchanter-ai lifecycle that produced golem

> Take-home PDF → **deterministic intake** → **roadmap decomposition** → **per-model prompt
> engineering** → **deep research** → **governed multi-agent build** → **adversarial hardening** →
> **CI validation** → **submission assembly** — every stage instrumented and verified.

| Phase | enchanter-ai product | What it did for golem |
|------|----------------------|-----------------------|
| 0 · Intake | *doc-ingest engine* | Converted the take-home **PDF → structured markdown** (`docs/assignment.md`) — the canonical spec every later phase referenced. |
| 1 · Decompose | **roadmap engine** | Split the whole effort into a **dependency-ordered roadmap** and routed each task to a **managed agent sized to its purpose** — Haiku for validation/completeness gates, Sonnet for execution (convergence, tests, conversions), Opus for judgment (architecture, technique selection). |
| 2 · Prompt-craft | **wixie** | Gave every agent a **model-matched prompt** (XML for Claude-family, the 5-axis DEPLOY bar, 8 SAT assertions, σ<0.45). wixie did not stop at a score: it **hardened, converged, and *sandbox-validated*** the load-bearing prompts — running parts in an isolated sandbox to confirm they produced the *expected* result, not merely plausible text. |
| 3 · Research | **Golden Ultra Deep Research** (Opus 4.8) | Ran the deep-research pipeline (fan-out search → fetch → **adversarial verify** → cited synthesis) for the **gold-standard architecture of a Python package wrapping a Go engine core**: `expr-lang/expr` vs `google/cel-go`; gopy vs CGo `c-shared`+cffi; the golden Go library layout; Arpeely intel. Every claim triangulated against ≥2 primary sources with confidence + freshness. |
| 4 · Govern | **vis** | The dependency-free **behavioral substrate** under which every agent ran: conduct modules (think-first, surgical edits, verification, **honest-numbers**, **doubt-engine**, **capability-fidelity**) and the **F-code failure taxonomy**. vis is *why* "a passing score ≠ production-grade" was caught and *why* the gopy dead-end triggered a HALT instead of a silent degrade. |
| 5 · Secure | **hydra** | The **security gate**: `gosec` + `govulncheck`, the G115 integer-overflow findings (guarded `uint→int64` conversions), and the standing "no secrets, no dangerous commands" veto at write/exec time. |
| 5 · Verify | **mimir** | **Provenance + verification.** Every agent's *claimed* completion was independently re-checked — "did it actually do it, or just say it?" — via adversarial-verify panels, honest gate-status tables, and a full **re-audit against the fixed code**. Nothing was trusted on assertion. |
| 5 · Review | **lich** | The **code-review pipeline**: dimension-by-dimension audits and the production-grade hardening rounds that surfaced the build-breaking errors a prose score had missed. |
| 5 · Structure | **gorgon** | Froze the repo's structure and decided the **golden flat-at-root Go layout** (rejecting the `pkg/`/`cmd/` project-layout anti-pattern for a single-package library, per the research). |
| 5 · Fidelity | **naga** | **Shape fidelity** to the enchanter-ai house style: the README rebuilt 1:1 against the canonical template, the `vis`-conduct import pattern, the modded-Minecraft creature-naming (`golem` ← Thaumcraft, *powered by Vis*). |
| 5 · Cost | **pech** | The **cost ledger**: token/spend attribution per agent and model, keeping the cheap tiers on mechanical work and reserving Opus for judgment. |
| 5 · Intent | **djinn** | **Drift control**: re-asserted the original goal at each checkpoint — the recurring "are we still standing against the PDF?" re-grounding that kept scope honest. |
| 5 · Flow | **crow** · **emu** · **sylph** | **crow** ordered change review by information gain; **emu** kept context healthy across a very long session (compress, checkpoint); **sylph** segmented the work into **logical commits** and pushed. |
| 6 · Observe | **beholder** | End-to-end **observability** over agent and CI execution — watching the GitHub Actions runs (Go gates + the c-shared wheel matrix) to ground every "green/red" claim in real run output. |

---

## Division of labor (the through-line)

- **Human** — judgment, architecture, and steering: the build-vs-buy ruling, the public-API shape,
  the **loud-by-default** stance, the **Python-first** product goal, the demand for *production-grade
  over deploy-score*, the direction to **pivot gopy → cffi**, and every locked decision and
  re-grounding ("are we standing against the PDF?").
- **robit (AI)** — execution under vis: deep research, code, table-driven / fuzz / concurrency /
  benchmark tests, the CGo `c-shared` + cffi binding, the CI and wheel pipelines, and the docs —
  all against the human's spec, with mimir verifying that claimed work was actually done and the
  human verifying correctness on a clean checkout.

## The load-bearing decisions

**1. Wrap, don't build.** Research (phase 3) chose **`expr-lang/expr`** over `google/cel-go`: a
single permissive **MIT** license, higher throughput, native `??` / `?.` / inline `let`; cel-go's
dual-implementation parity edge is moot under a single-core architecture. Recorded in
`docs/adr/0001-why-wrap-not-build.md`.

**2. One Go core, LOUD by default.** The human fixed the architecture: one **Go core** for the
millions/sec + thread-safe hot path; a typo'd variable is a **compile-time error**, never a silent
`0`. robit scaffolded it as a thin wrapper over `expr-lang` — compile-once into an immutable
`*vm.Program`, a bounded `golang-lru/v2` cache, a 7-type error API, a strict `expr.Env` schema, a
`safeRun` panic→error boundary, plus cost-budget and timeout guards.

**3. Production-grade, not deploy-score (the banked lesson).** An early pass scored well on a prose
rubric and *looked* done. The human pushed for **production-grade**; vis's doubt-engine +
verdict-calibration fired, and mimir's adversarial reviews + fact-checks caught **build-breaking
errors a quality score had missed** — e.g. that `expr` has **no VM-level preemption** (forcing the
goroutine-raced timeout and making `WithCostLimit` the primary guard), and the exact license /
thread-safety facts of the LRU dependency. **A passing quality score is not production-grade; only
an independent adversarial check against primary sources is.**

**4. Python-first for non-technical users.** The deliverable was reframed around a data scientist
who `pip install enchanter-golem`, `import golem`, and writes a formula in a notebook — never a Go
toolchain, never a goroutine — while a Go service embeds the *same* core for identical answers.

**5. The gopy → cffi pivot (capability-fidelity in action).** The original binding plan was
**gopy**. gopy **could not bind golem's `any`-rich API** (functional-option constructors,
`map[string]any` variable maps). Per vis's capability-fidelity rule, robit **HALTed and invoked the
pre-planned fallback** rather than silently degrade: a CGo `-buildmode=c-shared` library
(`libgolem`) exposing a **string-only C ABI** (`GolemNewEngine` / `GolemEvalJSON` /
`GolemFreeEngine` / `GolemFreeString`), loaded via **cffi** in ABI mode. Dynamic data crosses as a
type-tagged **JSON envelope** — which is *why* the string-only boundary is the right design.

**6. CI to green wheels.** The workflows iterated until they produced **prebuilt wheels across
Linux, macOS, and Windows** with the compiled Go engine baked in — `go build` / `go vet` /
`go test` / `go test -race` plus the Go↔Python parity corpus, the c-shared build, and a no-Go
`pip install` smoke test — all verified green by beholder against real run output.

## Verification (mimir)

Claims were not trusted on assertion. mimir re-checked each agent's output; the human verified
correctness independently on a **clean checkout** — the acceptance gates (`go build`, `go vet`,
`go test`, `go test -race` with many goroutines sharing one `Program`, the benchmark for real
cached-vs-cold `ns/op`, the `FuzzCompile` target for escaped panics, and the Go↔Python parity
corpus) — and adversarially fact-checked the load-bearing framework claims against primary sources
before accepting AI-generated code.

## Full circle

golem is itself a member of this ecosystem — the **deterministic expression primitive** the agent
products consume (wixie for cost formulas; robit/beholder for guardrail and routing rules; pech for
cost-rule expressions). It was built *by* the enchanter-ai stack and is built *for* it.

---

*A note on this record:* the products above are components of the enchanter-ai stack, all governed
by **vis** and orchestrated by **robit**; this document declares the lifecycle and discipline
applied at each phase. The authoritative, blow-by-blow record is robit's raw session transcript.
