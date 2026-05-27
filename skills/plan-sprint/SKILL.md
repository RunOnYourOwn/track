---
name: plan-sprint
description: >-
  Select and schedule work for the next sprint. Pulls from track backlog,
  respects capacity and dependencies, identifies parallel-safe task groups,
  outputs a sequenced sprint plan with agent-minute budget.
---

# Plan Sprint

Select work for the next sprint. No questions — run to completion.

## Input

Optional:
- `--capacity {minutes}` — override default (270 min = 3 sessions x 90 min)
- `--focus {task IDs}` — force-include specific tasks
- `--project {PREFIX}` — target project (defaults to current from CLAUDE.md)

## Steps

### 1. Load board state

```bash
track task list --project {PREFIX}
```

Categorize:
- **In progress**: carry-over (counts against capacity at remaining estimate)
- **Ready**: status `todo`, not blocked, has `estimate_agent_minutes`
- **Unsized**: status `todo`, no agent-minutes → flag for `/estimate` first
- **Blocked**: note blockers

### 2. Calculate capacity

Default: 3 sessions x 90 agent-minutes = 270 min/week
Buffer: 15% for context switching, reviews → effective = 230 min
Subtract carry-over estimates.

### 3. Select sprint work

Priority order:
1. Force-included tasks (from --focus)
2. In-progress carry-over
3. Urgent priority
4. High priority
5. Medium priority (fill remaining capacity)

Stop when total agent-minutes >= effective capacity.

### 4. Identify parallel groups

For each selected task, check file scope from description.
Group tasks into parallel batches:

- **Batch A**: tasks with no shared files → can run simultaneously via `/parallel-sprint`
- **Batch B**: tasks sharing files with Batch A → must run after Batch A merges
- **Sequential**: tasks with explicit `blocks` dependencies

Rules:
- Max 3 agents in parallel (resource limit)
- Tasks touching the same file cannot be in the same batch
- Dependencies override file analysis (blocked tasks always come after)

### 5. Sequence the sprint

Order by:
1. Dependencies (blockers first)
2. Parallel batches (run together)
3. Risk (higher risk earlier)
4. Priority (higher first within same tier)

Assign to sessions:
- Session 1: Batch A (parallel)
- Session 2: Batch B (parallel) or sequential follow-ups
- Session 3: remaining + buffer

### 6. Output sprint plan

```
## Sprint Plan — Week of {date}

**Capacity**: {minutes} min effective (3 sessions, 15% buffer)
**Carry-over**: {minutes} min from {N} tasks

### Session 1 — Parallel batch
| Task | Title | Agent min | Files | Notes |
|------|-------|-----------|-------|-------|
| {PREFIX}-4 | Fix calc | 30 | 2 | |
| {PREFIX}-6 | Add tab | 45 | 3 | |
→ Run with: `/parallel-sprint {PREFIX}-4 {PREFIX}-6`

### Session 2 — Sequential
| Task | Title | Agent min | Files | Notes |
|------|-------|-----------|-------|-------|
| {PREFIX}-8 | Metrics | 75 | 4 | depends on {PREFIX}-6 |

### Session 3 — Buffer / overflow
| Task | Title | Agent min | Files | Notes |
|------|-------|-----------|-------|-------|
| {PREFIX}-9 | Refactor | 45 | 2 | if time allows |

**Total committed**: {sum} min / {capacity} min
**Slack**: {remaining} min
**Parallel opportunities**: {N} tasks in {M} batches

### Not selected (deferred)
- {PREFIX}-12: {title} — {reason: capacity/blocked/unsized}

### Risks
- {any identified risks or file conflicts}
```

### 7. Start first batch

```bash
track task move {PREFIX}-{N} --status in_progress
```

If parallel batch identified, suggest:
```
Ready to execute. Run: /parallel-sprint {task-ids}
```
