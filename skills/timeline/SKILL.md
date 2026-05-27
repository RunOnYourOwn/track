---
name: timeline
description: Generate a multi-week timeline for a project or epic. Shows estimated completion dates, milestones, and critical path. Use for planning visibility and stakeholder communication.
---

# Timeline

Generate a timeline projection. No questions — run to completion.

## Input

Either:
- `--project {PREFIX}` — timeline for entire project backlog
- `--epic {TASK_ID}` — timeline for a specific epic and its subtasks
- No input — timeline for current project from CLAUDE.md

Optional:
- `--capacity {hours/week}` — override default (9h/week)
- `--start {date}` — start date (defaults to today)

## Steps

### 1. Load all tasks

```bash
track task list --project {PREFIX}
```

Filter to non-done tasks. Note estimates from task metadata.

### 2. Flag unsized tasks

If any tasks lack estimates, list them:
```
Warning: {N} tasks without estimates — run /estimate first for accuracy
```
Assign default M (1.5h) for projection purposes but mark as uncertain.

### 3. Calculate timeline

Parameters:
- Capacity: 9h/week effective (default) or override
- Sessions: 3/week x 3h
- Parallelism: 1 (sequential by default; note where parallel is safe)

Sequence tasks by:
1. Priority (urgent → low)
2. Dependencies (blocked items after their blockers)
3. Current status (in_progress first)

Assign to weeks based on cumulative hours fitting within weekly capacity.

### 4. Identify milestones

Group tasks into milestones if logical clusters exist:
- All bug fixes = "Stability milestone"
- Feature group = "Feature X complete"

### 5. Output timeline

```
## Timeline: {project/epic name}

**Generated**: {today}
**Capacity**: {hours}/week ({sessions} sessions)
**Total effort**: {sum hours}h
**Estimated completion**: {date} ({N weeks})
**Confidence**: {high|medium|low} ({reason})

### Week-by-week

| Week | Dates | Tasks | Hours | Milestone |
|------|-------|-------|-------|-----------|
| 1 | May 27 - Jun 1 | {PREFIX}-3, {PREFIX}-5 | 8.5h | — |
| 2 | Jun 2 - Jun 8 | {PREFIX}-8, {PREFIX}-9 | 7.0h | Dashboard complete |

### Critical path
{task} → {task} → {task} (longest dependency chain: {hours}h)

### Risks to timeline
- {N} unsized tasks (estimated at default M)
- {task} is XL and may need decomposition
- {task} blocked on external dependency

### Parallel opportunities
- Week 1: {PREFIX}-3 and {PREFIX}-5 can run simultaneously (different files)
```
