---
name: onboard-project
description: >-
  Guided project onboarding — discovers existing work (git history, docs, backlogs),
  creates Track project, defines hierarchy, imports full task history, backdates timestamps,
  sets up CLAUDE.md. Interactive: confirms decisions before executing.
  Run from the target project directory.
---

# Onboard Project

Guided walkthrough to bring a project into the track system. **This skill is interactive** — confirm with the user before creating or modifying anything.

## Input

Either:
- A project prefix + name: `MYPROJ "My Project Name"`
- No input — onboard current project (derive prefix from directory name)

Run from the target project directory (create and `cd` into it first if new).

---

## Phase 1: Discovery

### 1a. Check for existing work (FULL HISTORY)

The goal is to capture the **complete project timeline** — past, present, and future — not just the current backlog. This gives stakeholders a full picture of velocity, decisions, and remaining scope.

Sources to scan for completed, in-progress, and planned work:
- `docs/sprint-tasks.md` — active task queue
- `docs/sprint-tasks-archive.md` — completed tasks with evidence
- `docs/session-notes/` — ALL session notes (narrative of what was built and when)
- `CLAUDE.md` — next steps, phase info, backlog/todo sections
- `git log --oneline --since="6 months ago"` on each repo — commits reveal completed work not in docs (gauge scale with `git log --oneline | wc -l` first; for large repos, focus on merge commits or tags)
- Issue tracker exports, README TODOs

**Key principle**: Every significant piece of work that was done, is being done, or is planned should appear as a task in track. Completed work gets marked `done` immediately after creation — this builds the velocity history. Relationships between tasks (`--blocks`) must be captured to show the dependency graph.

### 1b. Understand project scope

From README, CLAUDE.md, or directory structure, determine:
- What the project does
- Current phase (discovery, build, maintenance)
- Rough size
- Tech stack (language, frameworks)

### 1c. Present findings

**Confirm with user:**
```
I found the following:
- [Project description / scope]
- [N items from existing backlog sources]
- [Git history: N commits across M repos since DATE]

I'd suggest prefix "[PREFIX]" and name "[Name]".

Does this look right, or would you like to adjust?
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
  track project create --prefix {PREFIX} --name "{Name}" --phase-type build

Proceed?
```

Then execute.

### 2c. Create onboarding meta-tasks

After creating the project, immediately create tasks to track the onboarding process itself. These make onboarding resumable (if context runs out mid-import) and provide an audit trail.

```bash
track task create --project {PREFIX} --title "Discover existing work & resources" --type task --priority urgent
track task create --project {PREFIX} --title "Define epic/feature structure" --type task --priority urgent
track task create --project {PREFIX} --title "Import task history" --type task --priority urgent
track task create --project {PREFIX} --title "Set up local structure" --type task --priority urgent
track task create --project {PREFIX} --title "Verify and start server" --type task --priority urgent
```

Mark the first task done immediately (discovery is already complete by this point). Mark each subsequent task done as its phase completes. If the session is interrupted, the next agent can check which onboarding tasks are still `todo` to know where to resume.

These tasks can optionally live under a "Project Setup" feature — create one if the project has more than a trivial number of other tasks being imported.

---

## Phase 3: Define Structure

### 3a. Propose epics (release milestones)

**Epics are time-based release milestones** — they define *when* something ships, not *what* area it covers. Think POC, MVP, Production, v2.0 — not "Frontend" or "Data Pipeline". Features under an epic describe the capabilities delivered in that release.

For full-history imports, include completed milestones (marked done immediately) so velocity tracking has the full picture.

Based on discovery, propose epics (2-4 typical):

**Confirm with user:**
```
Based on the project timeline, here's the release structure:

1. Epic: "POC — [what was proved]" (done)
2. Epic: "MVP — [what shipped]" (done)
3. Epic: "Production — [what's next]" (active)

Should I create these, adjust them, or skip epics for now?
```

### 3b. Propose features under each epic

For each confirmed epic, suggest features:

**Confirm with user:**
```
Under "POC — end-to-end proof", I'd suggest these features:
1. "Data ingestion pipeline"
2. "Core transform logic"
3. "Basic validation"

Add, remove, or adjust any?
```

### 3c. Map full project history into tasks

Import ALL work — past, present, and future — into the task hierarchy. For large projects (20+ items), batch in groups of ~10 and confirm each batch:

**Confirm with user:**
```
I'll create tasks in batches. First batch — completed work (N items):
- "Implemented X" (done, 2h est, high)
- "Set up Y pipeline" (done, 4h est, urgent)
- ...

Proceed with this batch?
```

**Import rules:**

1. **Completed work** (from archive, session notes, git log):
   - Create tasks with descriptive titles and evidence (commit hashes, PR links)
   - Immediately mark as `done` after creation
   - Place under the correct epic/feature

2. **In-progress work** (from sprint-tasks.md `in_progress` items):
   - Create and leave as `todo` (track doesn't have in_progress on create)
   - Move to in_progress: `track task move ID --status in_progress`

3. **Planned/pending work** (from sprint-tasks.md `pending`, session notes "next steps"):
   - Create as `todo` with full descriptions

4. **Blocked work** (from sprint-tasks.md `blocked` items):
   - Create as `todo`, note blocker in description
   - If blocked by another task, add `--blocks` link

5. **Dependency links** — capture relationships:
   - Backend → Frontend (backend must deploy before frontend can use it)
   - Schema changes → builds that validate them
   - Infra/config → features that depend on them
   - Sequential pipeline stages
   - Use: `track task link BLOCKER-ID --blocks BLOCKED-ID`

**Carry forward metadata**: Set `--priority` and `--estimate` from source docs where available. Don't leave imported tasks unsized if the source already had sizing.

**Do NOT skip completed work.** The full history is what makes velocity reports and timeline projections meaningful.

### 3d. Backdate timestamps from git history

If the project directory is a git repo (or contains git repos), use commit dates to set accurate `created_at` and `completed_at` timestamps on imported tasks. Track creates all tasks with "now" as the timestamp — without backdating, velocity reports and timeline projections are meaningless for historical work.

**Steps:**

1. Pull git log with dates from each repo:
   ```bash
   git log --format="%ad %s" --date=short --since="6 months ago"
   ```

2. Match commits to tasks by:
   - Task IDs in commit messages (e.g. `feat(SME-004): ...`)
   - Feature descriptions matching task titles
   - Tag/release dates for milestone boundaries

3. Update timestamps directly in SQLite (track has no `task edit` command):
   ```sql
   -- Completed tasks: set both created_at and completed_at
   UPDATE tasks SET 
     created_at = '2026-05-08T08:00:00Z',
     completed_at = '2026-05-08T17:00:00Z'
   WHERE id = 'TASK-ULID';

   -- Pending tasks: set created_at to when they were first identified
   UPDATE tasks SET created_at = '2026-05-20T08:00:00Z'
   WHERE id = 'TASK-ULID';

   -- Epics: created_at = first activity, completed_at = last task done
   UPDATE tasks SET 
     created_at = '2026-05-03T08:00:00Z',
     completed_at = '2026-05-19T18:00:00Z'
   WHERE id = 'EPIC-ULID';
   ```

4. Use release tags (`git tag -l --sort=-creatordate --format='%(creatordate:short) %(refname:short)'`) as milestone boundaries for epic dates.

5. Verify with a velocity check:
   ```sql
   SELECT substr(completed_at, 1, 10) as day, COUNT(*) as tasks
   FROM tasks WHERE project_id = (SELECT id FROM projects WHERE prefix = 'PREFIX')
     AND status = 'done' AND completed_at IS NOT NULL
   GROUP BY day ORDER BY day;
   ```

**Skip this step** if the project has no git history (brand new project).

---

## Phase 4: Set Up Local Structure

### 4a. Create/update CLAUDE.md

**Confirm with user:**
```
I'll create/update CLAUDE.md with:
- Taskboard section (prefix + ID)
- Project metadata
- Session metadata

Proceed?
```

Minimum CLAUDE.md additions:

```markdown
## Taskboard
Project: {PREFIX} (ID: {ulid})

## Session
Latest: {today}
Phase: {detected phase}
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

### 4c. Archive superseded docs

Once all tasks are imported into track, move redundant task-tracking files into `docs/archive/`:

```bash
mkdir -p docs/archive
mv docs/sprint-tasks-archive.md docs/archive/ 2>/dev/null
mv docs/blocked-tasks.md docs/archive/ 2>/dev/null
mv docs/dropped-tasks.md docs/archive/ 2>/dev/null
```

**Keep `docs/sprint-tasks.md` in place** — `/session-start` still reads it to surface ready/blocked tasks. Once the project is fully transitioned (after 1-2 sprints using track exclusively), archive it then.

**Track is now the source of truth** for task state. Archived files are kept for historical reference only. Do NOT delete them — archive preserves the record of how tasks were tracked before onboarding.

---

## Phase 5: Start Track Server

```bash
track server-status || track serve
```

---

## Phase 6: Output Summary

```
## Onboarded: {Project Name} ({PREFIX})

### Created
- Track project: {PREFIX}
- Epics: {N}
- Features: {N}
- Tasks: {N} ({M} with estimates, {K} need sizing)
- CLAUDE.md [created | updated]
- docs/session-notes/current.md

### Next steps
1. /estimate — size the {K} unsized tasks
2. /plan-sprint — select work for this week
3. /session-start — begin first tracked session

Server: http://localhost:3011
```
