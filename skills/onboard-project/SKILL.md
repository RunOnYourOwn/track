---
name: onboard-project
description: >-
  Guided project onboarding — walks the user through creating a project in track,
  defining epics and features, importing existing backlog, and setting up CLAUDE.md.
  Interactive: confirms decisions before executing. Run from the target project directory.
---

# Onboard Project

Guided walkthrough to bring a project into the track system. **This skill is interactive** — confirm with the user before creating or modifying anything.

## Input

Either:
- A project prefix + name: `MYPROJ "My Project Name"`
- No input — onboard current project (derive prefix from directory name)

Run from the target project directory.

---

## Phase 1: Discovery

### 1a. Check for existing work

Look for backlog sources:
- `docs/sprint-tasks.md` — task tables
- `CLAUDE.md` backlog/todo sections
- `docs/session-notes/` — recent session context
- Issue tracker exports, README TODOs

### 1b. Understand project scope

From README, CLAUDE.md, or directory structure, determine:
- What the project does
- Current phase (discovery, build, maintenance)
- Rough size

### 1c. Present findings

**Confirm with user:**
```
I found the following existing work:
- [N items from sprint-tasks.md]
- [M items from CLAUDE.md backlog]
- [session notes from date X]

I'd suggest prefix "[PREFIX]" and name "[Name]".

Does this look right, or would you like to adjust the prefix/name?
```

Wait for confirmation before proceeding.

---

## Phase 2: Create Track Project

### 2a. Check if project exists

```bash
track project list
```

If it already exists, ask whether to use the existing one or create fresh.

### 2b. Propose and confirm

**Confirm with user:**
```
I'll create the project:
  track project create {PREFIX} "{Name}" --methodology build

Proceed?
```

Then execute.

---

## Phase 3: Define Structure

### 3a. Propose epics (release milestones)

Based on discovery, propose 2-4 epics:

**Confirm with user:**
```
Based on what I see, here's a suggested release structure:

1. Epic: "Phase 1 - Core functionality" (urgent)
2. Epic: "Phase 2 - Polish + integrations" (high)
3. Epic: "Phase 3 - Production readiness" (medium)

Should I create these, adjust them, or skip epics for now?
```

### 3b. Propose features under each epic

For each confirmed epic, suggest features:

**Confirm with user:**
```
Under "Phase 1 - Core functionality", I'd suggest these features:
1. "Data ingestion pipeline"
2. "Validation rules engine"
3. "Basic dashboard"

Add, remove, or adjust any?
```

### 3c. Import existing tasks

If backlog items were found, map them to the hierarchy:

**Confirm with user:**
```
I'll import these [N] existing items as tasks:
- "Fix chart rendering" → under "Dashboard improvements" (in_progress)
- "Add retry logic" → under "Data pipeline" (todo)
- ...

Look right?
```

Then execute the creates/moves.

---

## Phase 4: Set Up Local Structure

### 4a. Confirm CLAUDE.md changes

**Confirm with user:**
```
I'll add a Taskboard section to your CLAUDE.md:

## Taskboard
Project: {PREFIX} (ID: {ulid})

And create docs/session-notes/current.md for session state.

Proceed?
```

### 4b. Create session notes

Create `docs/session-notes/current.md`:

```markdown
# Current Session

**Date:** {today}
**Phase:** {detected phase}

## Status
Project onboarded into track. Backlog imported.

## Next steps
1. Review imported task hierarchy
2. Run /estimate on unsized tasks
3. Run /plan-sprint to start first sprint
```

---

## Phase 5: Output Summary

```
## Onboarded: {Project Name} ({PREFIX})

### Created
- Track project: {PREFIX}
- Epics: {N}
- Features: {N}
- Tasks: {N} ({M} with estimates, {K} need sizing)
- CLAUDE.md Taskboard section
- docs/session-notes/current.md

### Next steps
1. /estimate — size the {K} unsized tasks
2. /plan-sprint — select work for this week
3. /session-start — begin first tracked session
```
