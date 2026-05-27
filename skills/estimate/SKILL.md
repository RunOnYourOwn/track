---
name: estimate
description: Estimate effort for track tasks. Sizes with T-shirt + hours. Run on individual tasks or bulk-estimate a project's backlog.
---

# Estimate

Size tasks with effort estimates. No questions — run to completion.

## Input

Either:
- A task ID (e.g. `PROJ-5`) — estimate that task
- `--project {PREFIX}` — bulk-estimate all unsized tasks in the project
- No input — estimate all unsized tasks in the current project (from CLAUDE.md Taskboard section)

## Steps

### 1. Gather tasks to estimate

For single task:
```bash
track task get {PREFIX}-{N}
```

For bulk:
```bash
track task list --project {PREFIX}
```
Filter to tasks without an estimate size.

### 2. Analyze each task

For each task without an estimate:
- Read the title and description
- If file scope is listed, check file sizes and complexity
- If no file scope, explore codebase to determine scope
- Consider: number of files, logic complexity, test requirements, risk of regressions

### 3. Apply sizing model

| Size | Hours | Typical scope |
|------|-------|---------------|
| XS | 0.25-0.5 | Config change, typo fix, single-line edit |
| S | 0.5-1 | One file, straightforward logic, clear pattern to follow |
| M | 1-2 | 2-4 files, moderate logic, some test writing |
| L | 2-3 | 3-6 files, significant logic, cross-cutting concerns |
| XL | 3-4 | 5+ files, complex, high risk — flag for decomposition |

Complexity modifiers (add 25-50%):
- Touches shared infrastructure or context providers
- No existing test coverage in the area
- Cross-module state management
- Performance-sensitive code path

### 4. Update tasks

Note: track stores estimates as task metadata set at creation. For existing tasks without estimates, record findings and create replacement tasks if needed, or note estimates in output for manual reference.

### 5. Output summary

```
## Estimates

| Task | Title | Size | Hours | Complexity | Notes |
|------|-------|------|-------|------------|-------|
| {PREFIX}-5 | Fix availability calc | S | 0.75 | standard | Single function fix |
| {PREFIX}-8 | Add Metrics tab | L | 2.5 | elevated | New component + data fetch |

**Total backlog**: {sum hours}h
**Sessions needed**: {ceil(sum / 3)} (at 3h/session)
**XL tasks needing decomposition**: {list or "none"}
```
