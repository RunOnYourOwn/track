# Skills

Claude Code skills for AI-assisted project management with Track. These are structured prompts that teach your LLM agent how to use track effectively.

## Setup

Copy the skill folders into your Claude Code skills directory:

```bash
# Copy all skills
cp -r skills/*/ ~/.claude/skills/

# Or copy specific ones
cp -r skills/session-start ~/.claude/skills/
cp -r skills/tdd ~/.claude/skills/
```

Each skill is a folder containing `SKILL.md` (the prompt) and optionally helper scripts.

## Skills Reference

### Onboarding

| Skill | Invoke | Purpose |
|-------|--------|---------|
| `onboard-project` | `/onboard-project` | Guided project setup. Discovers existing work, proposes structure, confirms before creating. |

### Daily Session

| Skill | Invoke | Purpose |
|-------|--------|---------|
| `session-start` | `/session-start` | Orient at session start. Reads board state, repo status, session notes. |
| `session-end` | `/session-end` | Close session. Updates tasks, logs work, writes session notes. |

### Planning & Execution

| Skill | Invoke | Purpose |
|-------|--------|---------|
| `plan` | `/plan` | Plan a feature. Explores codebase, produces decomposed task list. |
| `decompose` | `/decompose` | Break a feature into sized implementation tasks. |
| `estimate` | `/estimate` | Size tasks with T-shirt estimates and hours. |
| `plan-sprint` | `/plan-sprint` | Select and schedule work for the next sprint. |
| `parallel-sprint` | `/parallel-sprint` | Execute multiple tasks concurrently in git worktrees. |
| `timeline` | `/timeline` | Forecast completion dates for stakeholders. |
| `tdd` | `/tdd` | Test-driven development methodology reference. |

### Reporting & Quality

| Skill | Invoke | Purpose |
|-------|--------|---------|
| `project-status` | `/project-status` | Cross-project dashboard with velocity and health. |
| `weekly-report` | `/weekly-report` | End-of-week summary for stakeholder visibility. |
| `deep-audit` | `/deep-audit` | Multi-phase whole-codebase audit with verification. |

## Workflow Scenarios

### 1. New Project (one-time)

```
/onboard-project        # Guided setup: project + epics + features
/decompose PROJ-4       # Break first feature into tasks
/estimate               # Size unsized tasks
/plan-sprint            # Schedule this week's work
```

### 2. Existing Project (one-time)

```
/onboard-project        # Import from sprint-tasks.md, CLAUDE.md, ADO
/estimate               # Size imported tasks
/plan-sprint            # Plan first sprint
```

### 3. Active Project (daily loop)

```
/session-start          # Orient: board state, blockers, next task
# ... work ...
/session-end            # Log time, update notes, write next steps
```

### 4. Weekly Cadence

```
/plan-sprint            # Monday: plan the week
/parallel-sprint        # Execute parallel-safe tasks
/weekly-report          # Friday: summarize for stakeholders
```

### 5. As-Needed

```
/decompose PROJ-12      # When next feature is ready to build
/timeline               # When stakeholders ask "when?"
/deep-audit             # Before milestones or releases
/project-status         # For standups or check-ins
```

## Requirements

- Track CLI installed and on PATH
- A project created (`track project create PROJ "Project Name"`)
- `docs/session-notes/current.md` for session state (created by session-end)
- CLAUDE.md with a `## Taskboard` section naming the project prefix

## Customization

These skills reference track CLI commands. If your project uses a different structure, edit the skill files to match. The skills are designed to be self-contained — each one runs without pausing for questions.

## Visual Workflow Guide

Open `docs/workflow.html` in a browser for an interactive visual guide showing all three scenarios with detailed steps.
