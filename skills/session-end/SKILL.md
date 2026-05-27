---
name: session-end
description: Close out a session. Updates track task states, logs work, writes session notes, updates CLAUDE.md. Universal across all projects.
---

# Session End

Run all steps without pausing. No questions.

## Steps

### 1. Identify work done this session

Review the conversation to determine:
- Which tasks were worked on (by task ID or by matching work to task titles)
- Which tasks were completed (all subtasks done, code committed, tests passing)
- Any new work discovered that needs tasks created
- Any blockers encountered

### 2. Update track

Run the following shell commands as appropriate:

**Tasks completed:**
```bash
track task done {PREFIX}-{N}
```

**Tasks started but not finished:**
Ensure they are `in_progress`:
```bash
track task move {PREFIX}-{N} --status in_progress
```

**New follow-up work discovered:**
```bash
track task create --project {PREFIX} --title "{title}" --priority {priority}
```

**Log time (if not already logged):**
```bash
track session log {PREFIX}-{N} --hours {estimate} --note "{what was done}"
```

### 3. End the track session

```bash
track session end --summary "{1-2 sentence summary of work done}"
```

### 4. Write session notes

Update `docs/session-notes/current.md` with the **minimal format**:

```markdown
# Current Session

**Date:** [today]
**Branch:** [active branch]
**Phase:** [current phase — update if changed]

## Status
[1-2 sentence summary of what was accomplished, referencing task IDs]

## Next
[PREFIX]-[N] — [title of highest priority next task]
```

If significant work was done, also write a dated archive:
`docs/session-notes/YYYY-MM-DD.md` with:
- Date and branch
- Tasks completed (with IDs)
- Tasks in progress (with progress)
- Tasks created
- Key decisions or discoveries

### 5. Update CLAUDE.md

- Set `Latest:` to today's date
- Update `Phase:` line if it changed
