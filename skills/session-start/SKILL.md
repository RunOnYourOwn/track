---
name: session-start
description: Orient to a project at session start. Reads CLAUDE.md, queries track for board state, checks repos. Universal across all projects. No questions asked.
---

# Session Start

Run all steps without pausing. No questions.

## Steps

1. Read `CLAUDE.md` in the current working directory. Extract:
   - Project name
   - Repo names (for step 4)
   - Current phase
   - Track project prefix (from `## Taskboard` section)

2. Read `docs/session-notes/current.md` — contains last session date, active branch, and phase. If it doesn't exist, list `docs/session-notes/` and read the most recent dated file instead.

3. **Query track board state**:
   ```bash
   track task list --project {PREFIX} 2>/dev/null
   ```
   If track is not installed or project doesn't exist, fall back to checking `docs/sprint-tasks.md` (legacy mode).

   Parse the output into categories:
   - **In Progress**: tasks with status `in_progress`
   - **Todo (ready)**: tasks with status `todo`
   - **Blocked**: any task whose title contains "blocked"

   Sort by priority: urgent > high > medium > low.

4. For each repo in the project:
   - Run `git -C ./<repo-name> status --short` — surface any uncommitted changes

5. Check for active sprint:
   ```bash
   track sprint list {PREFIX} 2>/dev/null
   ```

## Output

```
## Session: [project name] — [today's date]

**Phase**: [from current.md]
**Last session**: [date from current.md] — [1-sentence summary]
**Active branch**: [from current.md]

### Repos
- [repo-name]: uncommitted changes: [none | list files]

### Board ([PREFIX])
In Progress:
  [PREFIX]-[N]  [title]  [priority]

Todo — Ready (N):
  [PREFIX]-[N]  [title]  [priority]

Blocked (N):
  [PREFIX]-[N]  [title]  ← [blocker reason]

### Active sprint
[sprint name]: [N tasks, Xh remaining]

### Suggested next steps
1. [highest priority in-progress task to continue]
2. [highest priority ready task to start]
3. [second ready task or blocker to resolve]
```

## End-of-Session Reminder

When the session ends, run `/session-end` to update task states and write session notes.
