---
name: deep-audit
description: Use when the user wants a deep, thorough, whole-codebase audit/review of a project's CURRENT STATE (not a diff) — e.g. at project start, at a milestone, or before a known breaking change. Runs a structured, multi-phase, multi-agent fan-out audit that persists findings to disk and verifies them. NOT for reviewing a diff/PR (use /review or /ultrareview for that).
---

# Deep Audit

A structured, repeatable, whole-codebase audit. Fans out many agents for breadth,
adds specialized agents for depth, derives the project's high-risk areas at
runtime (so it is **generic across projects**), and — crucially — **verifies**
every High/Critical finding before it lands in the report (the step that turns a
noisy 200-finding dump into high signal).

**When to use:** project start, milestones, before a breaking change, periodic
health checks (realistically 1–4× over a project's lifecycle — it takes hours).
**When NOT to use:** reviewing a single change/PR → that's `/review` (local) or
`/ultrareview` (cloud, verified, diff-scoped).

**Why multi-phase + verify:** single-pass review is <50% actionable; fan-out
breadth + aspect depth gets coverage; an independent verify pass is what keeps
false positives near zero. (See the method's roots: fan-out auditing + verified
multi-agent review.)

## Project-agnostic by design (lazy-load specifics)

Nothing here is hardcoded to one project. Per-project specifics are loaded **at
runtime**, in this precedence:

1. **Repo audit config (optional):** if `<repo>/.deep-audit.md` exists, read it —
   it may declare focus areas, known-risky modules, invariants to check, paths to
   exclude, and an `accepted:` list of known-and-intentional risks. Pass the
   `accepted:` list to the Phase-1 agents: anything matching it is filed as a
   single **Low** "accepted risk (see .deep-audit.md)" line, NOT re-derived as a
   Medium/High each run (this is what stops infra/config slices from drowning the
   report in known items). Offer to scaffold one (template at the end) if absent.
2. **Repo conventions:** read `<repo>/AGENTS.md` and/or `CLAUDE.md` if present —
   their documented invariants/rules become explicit audit checks.
3. **Runtime discovery (Phase 0):** the architecture map derives the rest — the
   project's real high-risk areas and the aspect list — from the code itself.

So a fresh project with none of (1)/(2) still gets a full audit from (3) alone.

## Output

Everything persists to `<repo>/docs/audits/<YYYY-MM-DD>/` (run `date +%F` — never
guess). **If that dir already exists (a prior run today — e.g. a re-audit right
after remediation), write to `<repo>/docs/audits/<YYYY-MM-DD>-rN/` (next free N:
`-r2`, `-r3`, …) so you never clobber a prior report; note the prior dir as the
one to diff against.** Findings live on disk (not in chat context), so the run
is resumable and each audit is diffable against the previous one. Layout:

```
docs/audits/<date>/
  phase0/MAP.md            # architecture + threat surface + derived aspect list
  phase1/slice-NNN.md      # one file per fan-out slice
  phase2/<aspect>.md       # one file per depth dimension
  phase3/architecture.md   # cross-cutting + invariant findings
  REPORT.md                # final, verified, severity-ranked
  PROCESS-NOTES.md         # how to improve THIS skill next time (self-tuning)
```

## Finding format (every agent uses this — keep it parseable)

```
### [Critical|High|Medium|Low] <short title>
- **Where:** path:line(s)
- **What:** the concrete issue
- **Why it matters:** impact / failure mode
- **Verify:** exact command or code path to reproduce/confirm
- **Fix:** suggested direction (don't implement — this is audit-only)
```

Severity: **Critical** = data loss / security hole / crash of a core loop;
**High** = wrong behavior on a real path; **Medium** = edge case / latent risk;
**Low** = maintainability / minor. Report uncertainty honestly — a guess is a
Low, not a High.

## The phases

> Models: use **Sonnet** for breadth (Phase 1) and **Opus** for cross-cutting +
> verification (Phases 3–4). Dispatch Phase-1 agents in **parallel batches of
> ~8–10** (multiple Task calls per message). This is the multi-hour bulk; it is
> resumable because every agent writes to disk.

### Phase 0 — Map & inventory (1 pass, Opus)
Establish the baseline the deep agents reference. Produce `phase0/MAP.md` with:
- **Architecture map:** the main components/modules and how they connect.
- **Threat & failure surface:** every trust boundary and external edge — auth
  points, public endpoints/webhooks, untrusted input, secrets, network/IO calls,
  background processes/concurrency, persistence, third-party integrations.
- **High-risk areas:** where bugs would hurt most (core loops, money/data paths,
  concurrency, anything recently/heavily changed per `git log`).
- **The derived aspect list for Phase 2** = the generic dimensions below, minus
  any that don't apply, plus project-specific ones this map surfaced, plus
  anything from `.deep-audit.md`.

### Phase 1 — Fan-out breadth (many parallel agents, Sonnet)
1. Build the slice manifest deterministically:
   `bash <skill_dir>/scripts/plan_slices.sh <repo> docs/audits/<date>/slices.tsv 6 400`
   (size-aware: big files get a solo slice, doc `*.md` excluded; pass limiting
   pathspecs as extra args, or honor excludes from `.deep-audit.md`).
2. Dispatch agents to cover the slices: **one agent per slice, OR one agent per
   small contiguous RANGE of slices** (~6–8 slices ≈ ~20 files each — the range
   form is far more orchestratable for a single driver and loses little depth).
   Each agent **deep-reads its slice files** against the checklist and writes a
   SEPARATE `phase1/slice-NNN.md` per slice (the literal slice id), with line
   numbers, using the finding format. Empty slice → write "No findings" so
   resumption knows it ran.
   Checklist: correctness & logic errors; unhandled errors / swallowed failures;
   concurrency & races; resource leaks (connections, processes, file handles);
   input validation & injection; auth/authz at this layer; off-by-one / edge
   cases / nil handling; misuse of the project's own APIs; dead/duplicated code.
   **Severity discipline:** before filing a High/Critical, confirm the FULL path
   — the write actually happens, or the guard is actually absent (grep the
   suspected mitigation before asserting it's missing). A partial trace is a
   Medium at most. The Phase-4 verify pass exists, but don't offload all rigor
   onto it — a breadth agent that cries High on an unconfirmed trace wastes a
   verifier (run-2's "judge infinite-loop" was a false-positive: the counter was
   persisted two calls away from where the agent stopped reading).
3. Run in batches of ~8–10; persist as you go.

> **Self-improvement (every phase):** when an agent hits friction — the checklist
> missed a class of bug, a slice was too coarse/fine, an aspect overlapped
> another, the finding format didn't fit, a repro needed something it lacked —
> it appends a short note to `PROCESS-NOTES.md`. This is the feedback loop that
> tunes the skill over time; Phase 5 turns it into concrete edits.

### Phase 2 — Aspect depth (specialized agents, Opus/Sonnet)
One agent per dimension; each reads the relevant Phase-1 outputs + the real
source for its dimension and writes `phase2/<aspect>.md`. Default dimensions
(prune/extend from Phase 0 + config):
- **Security & trust boundaries** — authn/authz, secret handling, injection,
  SSRF/CSRF/XSS, the public endpoints from the map.
- **Concurrency, state & lifecycle** — races, locks, process/GenServer lifecycles,
  leases, idempotency, crash recovery.
- **Data & persistence** — schema/migration safety, query correctness, integrity,
  N+1s, transaction boundaries.
- **External integrations & failure modes** — timeouts, retries, partial failure,
  fail-open vs fail-closed, webhook/event handling.
- **API / web / UI layer** — routing, per-tenant/per-user scoping (IDOR),
  validation, error surfaces.
- **Build, deps & config** — supply chain, breaking deps, config drift, secrets in
  config, env assumptions.
- **Error handling & observability** — are failures logged, surfaced, recoverable?
- **Documented-invariant adherence** — each rule in AGENTS.md/CLAUDE.md becomes a
  check: is it actually upheld in code?

### Phase 3 — Architecture & invariants (1 pass, Opus)
Read all Phase 1+2 outputs + the map + AGENTS.md/plans. Assess what file-local
agents can't see: architectural integrity, cross-cutting duplication, invariant
violations, layering breaks, "half-finished" features, and design drift from the
project's stated principles. Write `phase3/architecture.md`.

### Phase 4 — Verify & rank (Opus) → REPORT.md
The signal step. For **every High/Critical** candidate from Phases 1–3, dispatch
a fresh agent to **independently confirm it**: re-read the code, trace the path,
and where feasible write/run a failing test or a concrete repro. **Also
sanity-check any library-default assumption** (e.g. "Req defaults to no timeout",
"this raises vs returns an error tuple") against the pinned dependency's actual
behavior — run-1 caught an overstated "timeout is infinite" claim this way. Mark
each **Confirmed / Unconfirmed / False-positive**; drop false positives, demote
unconfirmable ones. **The Phase-4 verdicts are the single source of truth for
severity and counts** — if the candidate list and the verdicts disagree (e.g. a
tally off-by-one), the report follows the verdicts and notes the reconciliation
visibly rather than silently. Then assemble `REPORT.md`:
- Executive summary (counts by severity, top themes, overall health verdict).
- Confirmed findings, ranked Critical → Low, each in the finding format with its
  verification result.
- An appendix linking the phase files.
Optionally also emit `findings.json` for tooling.

### Phase 5 — Process retro (self-improvement) → PROCESS-NOTES.md + skill edits
The skill audits itself, so each run is better than the last. Read the
`PROCESS-NOTES.md` notes accumulated across phases plus your own observations of
what worked and what didn't, and write a structured retro:
- **What worked / what was noise** — false-positive patterns, low-value checks,
  the breadth-vs-depth balance, model/effort choices, slice sizing, batch size.
- **Coverage gaps** — bug classes or areas the phases under-served; missing aspect
  dimensions; checklist items to add.
- **Concrete skill edits** — specific, actionable changes to `SKILL.md`,
  `plan_slices.sh`, the checklist, or the default aspect list (and to this
  project's `.deep-audit.md`). Phrase them as diffs-in-words ready to apply.
Then **propose those edits to the user** (don't silently rewrite the skill).
Applying them is a quick follow-up — over a few runs the skill converges on what
actually finds bugs in real projects.

## Guardrails
- **Audit-only — never modify project code.** Output is findings, not fixes. If
  the user wants fixes, that's a separate follow-up (file tasks from the report).
- **Verify before asserting.** An unverified suspicion is Low/Unconfirmed, never
  High. No style nits dressed up as bugs.
- **Persist everything to disk** as you go — the run is long; never hold findings
  only in context.
- **Stay read-only on outward systems.** Don't push, deploy, or hit external
  services; this is a local read of the code.

## Optional repo config — `.deep-audit.md` (scaffold if missing)
```markdown
# Deep Audit config
focus:        # extra dimensions / modules to scrutinize
  - <area>
invariants:   # project rules to verify are upheld
  - <rule>
risky:        # modules/paths known to be fragile
  - <path>
exclude:      # pathspecs to skip (beyond the script defaults; plan_slices.sh honors these)
  - <path>
accepted:     # known + intentional risks — file as a single Low, don't re-derive each run
  - <thing>: <one-line justification>
verify_cmds:  # how to run tests/build in THIS project (for Phase 4 repros)
  - <command>
```
