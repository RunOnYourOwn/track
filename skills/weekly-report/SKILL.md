---
name: weekly-report
description: Generate weekly progress report. Summarizes completed work, velocity, blockers, and next week plan across all projects. Run at end of week for stakeholder visibility.
---

# Weekly Report

Generate end-of-week summary. No questions — run to completion.

## Steps

### 1. Gather data from all projects

```bash
track project list
```

For each project:
```bash
track task list --project {PREFIX}
track velocity --project {PREFIX} --weeks 1
```

### 2. Read session notes

Check `docs/session-notes/` for this week's dated files (Mon–Sun). Extract:
- Key accomplishments per session
- Decisions made
- Blockers encountered and resolved
- New work discovered

### 3. Get velocity from track

```bash
track velocity --project {PREFIX} --weeks 4
```

This gives rolling velocity data computed from the database.

### 4. Output report

```
## Weekly Report — Week of {Monday date}

### Summary
{2-3 sentence executive summary of the week's progress}

### Completed ({N} tasks, {H}h estimated)

**{Project 1}**
- {ID}: {title}
- {ID}: {title}

**{Project 2}**
- {ID}: {title}

### In Progress (carrying over)
- {ID}: {title} — {progress note}

### Blocked
- {ID}: {title} — {blocker and who/what can unblock}

### Velocity
| Metric | This week | 4-week avg |
|--------|-----------|------------|
| Tasks | {N} | {avg} |
| Hours (est) | {H} | {avg} |
| Sessions | {S} | {avg} |

### Decisions & discoveries
- {key decision and rationale}
- {unexpected finding}

### Next week plan
1. {highest priority carry-over}
2. {next ready task}
3. {blocker to resolve}
```

### 5. Archive report

Write to `docs/reports/weekly-{YYYY-Www}.md` (create `docs/reports/` if needed).
