---
name: decompose
description: >-
  Decompose a high-level task into subtasks. Explores codebase, creates track
  subtasks with agent-minute estimates and file scope. Use after creating an
  epic or feature.
---

# Decompose

Break a task into implementable subtasks. No questions — run to completion.

## Input

Either:
- A task ID (e.g. `PROJ-12`) — decompose that task
- A description — create a parent task then decompose it
- No input — find the highest-priority `todo` task without subtasks (by priority field: urgent > high > medium > low, then lowest seq number)

## Steps

### 1. Identify the target

```bash
track task get {PREFIX}-{N}
```

If input was a description, create the parent first:
```bash
track task create --project {PREFIX} --title "{title}" --priority {priority} --type feature
```

### 2. Explore the codebase (read-only)

Based on the task's scope:
- Locate files that will need changes
- Understand current implementation and test coverage
- Identify shared dependencies and contracts
- Note files that must NOT be touched

### 3. Decompose into subtasks

Rules:
- Each subtask completable in one agent session (see sizing below)
- Each subtask independently testable
- Explicit file scope (list specific files, no wildcards)
- Include at least one acceptance criterion per subtask
- Note conflicts between subtasks touching the same file
- Identify which subtasks can run in parallel (no shared file edits)

### 4. Size with agent-minutes

| Size | Agent minutes | Typical scope |
|------|---------------|---------------|
| XS | 1-5 | Config change, typo fix, single-line edit |
| S | 5-30 | One file, straightforward logic, clear pattern to follow |
| M | 30-60 | 2-4 files, moderate logic, some test writing |
| L | 60-90 | 3-6 files, significant logic, cross-cutting concerns |
| XL | 90+ | 5+ files, complex, high risk — flag for further decomposition |

XL tasks should be decomposed further. Max task size is L (90 min).

### 5. Create subtasks in track

For each subtask:
```bash
track task create --project {PREFIX} --title "{title}" --priority {priority} --estimate {XS|S|M|L} --agent-minutes {5-90} --hours {0.5-4} --type task --parent {PARENT_TASK_ID} --description "{problem + acceptance criteria + file scope}"
```

If subtasks have dependencies:
```bash
track task link {PREFIX}-{N} --blocks {PREFIX}-{M}
```

### 6. Output summary

```
## Decomposed: {PARENT_ID} — {title}

| # | Subtask | Size | Agent min | Files | Parallel? |
|---|---------|------|-----------|-------|-----------|
| 1 | {ID} {title} | M | 45 | 3 | yes |
| 2 | {ID} {title} | S | 15 | 1 | yes (no shared files with #1) |
| 3 | {ID} {title} | L | 75 | 4 | no (depends on #1) |

**Total estimate**: {sum agent minutes} min ({N} sessions)
**Parallel safe**: {which subtasks can run simultaneously}
**Sequential**: {which must be ordered, and why}
**Sprint capacity note**: 3 sessions/week x 90 min = 270 min/week
```
