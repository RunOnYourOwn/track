---
name: project-status
description: Cross-project status dashboard. Shows all track projects with task counts, velocity, blockers, and health. Use for weekly standups or quick orientation across all active work.
---

# Project Status

Cross-project dashboard. No questions — run to completion.

## Input

Optional:
- `--project {PREFIX}` — single project deep-dive instead of cross-project view

## Steps

### 1. List all projects

```bash
track project list
```

### 2. For each project, get board state

```bash
track task list --project {PREFIX}
```

Calculate:
- Total tasks
- By status: done / in_progress / todo
- By priority distribution
- Blocked count

### 3. Get velocity data

```bash
track velocity --project {PREFIX} --weeks 4
```

### 4. Get health report

```bash
track health --project {PREFIX}
```

### 5. Output dashboard

**Cross-project view:**
```
## Project Status — {today}

| Project | Total | Done | In Progress | Todo | Blocked | Health |
|---------|-------|------|-------------|------|---------|--------|
| PROJ | 15 | 3 | 2 | 8 | 2 | warn |
| DEMO | 8 | 1 | 1 | 6 | 0 | ok |

### Blockers across projects
- {PREFIX}-7: {title} — {blocker}

### This week's velocity
- Completed: {N} tasks / {H} hours

### Capacity allocation
- {PREFIX}: {%} ({hours}h/week)
```

**Single project deep-dive (--project):**
```
## {Project Name} — Deep Dive

### Board
In Progress ({N}):
  {ID}  {title}  {priority}  est: {hours}h

Todo — Ready ({N}):
  {ID}  {title}  {priority}  est: {hours}h

Blocked ({N}):
  {ID}  {title}  ← {blocker}

### Burndown
Total backlog: {hours}h
Completed: {hours}h
Remaining: {hours}h
At current velocity: {weeks} weeks to clear backlog

### Next actions
1. {highest priority ready task}
2. {blocker to resolve}
3. {unsized tasks to estimate}
```
