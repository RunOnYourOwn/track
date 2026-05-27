---
name: parallel-sprint
description: >-
  Execute multiple track tasks in parallel using isolated git worktrees.
  Each task gets its own agent and branch. Pulls tasks from track board,
  detects file overlap, spawns concurrent agents, consolidates results.
---

# Parallel Sprint

Execute tasks concurrently. Spawns one worktree agent per task, collects results, provides merge guidance.

## Input

`/parallel-sprint [task-ids...]` — run specific tasks, e.g. `/parallel-sprint PROJ-4 PROJ-6`
`/parallel-sprint ready` — run all `todo` tasks with no blockers in current project
`/parallel-sprint` — same as `ready`

## Steps

### 1. Load task queue from track

```bash
track task list --project {PREFIX}
```

Resolve the task list:
- If task IDs provided: use those
- If `ready` or no args: select all `todo` tasks that are not blocked

For each task, get full details:
```bash
track task get {PREFIX}-{N}
```

If no runnable tasks found: list blocked tasks and stop.

### 2. Load project config from CLAUDE.md

Extract:
- Test command (from test runners table or detect from project type)
- Base branch (current git branch)
- Commit format (default: `feat({task-id}): {title}`)

### 3. Detect file overlap

For each task, extract file scope from its description.
Compare file sets across tasks.

- No overlap → safe to run in parallel and auto-merge
- Overlap → warn which tasks share files, ask to confirm

### 4. Mark tasks in-progress

```bash
track task move {PREFIX}-{N} --status in_progress
```

### 5. Spawn agents in parallel

For each task, call Agent with:
- `isolation: "worktree"`
- Full task description as context (from track)
- Project context: test command, commit format, base branch
- TDD instruction: write test first, then implement
- Report back: task ID, files changed, test result, branch name, issues

All agents run concurrently.

### 6. Collect and report results

```
Task          Branch              Tests    Status
{PREFIX}-4    worktree-{id}       47 pass  done
{PREFIX}-6    worktree-{id}       49 pass  done (file overlap with #4)
{PREFIX}-8    worktree-{id}       FAILED   blocked — test failure
```

For each task, verify acceptance criteria from description. Mark each PASS/FAIL.

### 7. Consolidation run

After all agents complete:
1. Cherry-pick successful commits onto a single consolidation branch
2. Run full test suite
3. If pass: ready for push
4. If fail: bisect to find offending commit, exclude that task

### 8. Update track

For each successful task (passed consolidation):
```bash
track task move {PREFIX}-{N} --status done
```

For failed tasks: leave as `in_progress`, log failure reason.

### 9. Merge guidance

**No file overlap**: provide sequential merge commands
**File overlap**: manual rebase instructions with diff commands
**Test failures**: flag clearly, exclude from merge

### 10. Output summary

```
## Sprint Complete

| Task | Title | Agent min | Result | Branch |
|------|-------|-----------|--------|--------|
| {PREFIX}-4 | Fix calc | 30 | done | worktree-4 |
| {PREFIX}-6 | Add tab | 45 | done | worktree-6 |

**Completed**: {N}/{total} tasks
**Agent time**: {sum minutes} min
**Merge ready**: {branch name} — run `git push origin {branch}` to publish
```
