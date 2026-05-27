---
name: plan
description: Planning phase for any code change. Explores codebase read-only, produces a spec and decomposed task briefs. Creates tasks in track on approval. Run this before starting implementation.
---

# Plan

Planning phase. No file writes until the user approves the task list.

## Input

The user provides a description of the change: new feature, bug fix, refactor, removal, or schema change. If no description is given, check track for the highest-priority unsized task and plan that.

## Steps

### 1. Understand the change

Read the user's description. Identify:
- What type of change: feat / fix / refactor / remove / schema
- Which subsystem or area is affected
- Whether this is a single task or a multi-task initiative

### 2. Explore the codebase (read-only)

Do not write any files during this phase. Use Read, Grep, Glob, Bash (read-only commands) to:
- Locate the files that will need to change
- Understand the current implementation and tests
- Identify shared utilities, constants, and schema contracts that touch the area
- Check existing tests for the affected code
- Note any files that must NOT be touched

### 3. Write the spec

One paragraph, internal only:
- What problem this solves and why it matters
- What the change does (not how)
- What it explicitly does NOT do

### 4. Decompose into task briefs

Rules for decomposition:
- Each task must be completable by one agent in one session (1-3 hours)
- Each task must be independently testable
- Each task must have explicit file scope (no "and related files")
- Tasks that touch the same file should be noted as potentially conflicting
- Every task must include at least one test case in acceptance criteria

Use this format for each task:

```
### {title}

**Type**: feat | fix | refactor | test | chore | schema
**Size**: XS | S | M | L | XL
**Hours**: {0.5-4}
**Files**: {explicit list}
**Blocked by**: {dependency, if any}

**Problem**: {2-3 sentences: what is wrong or missing and why it matters}

**Acceptance criteria**:
- [ ] {specific behavioural outcome}
- [ ] Test: {input} → {expected output}

**Verification**: {exact test command}
```

T-shirt sizes:
- XS: < 30 min, single file, trivial change
- S: 30-60 min, 1-2 files, straightforward
- M: 1-2 hours, 2-4 files, some complexity
- L: 2-3 hours, 3-6 files, significant logic
- XL: 3-4 hours, 5+ files — consider further decomposition

### 5. Present for review

Output:
1. The spec (one paragraph)
2. The full task list as a markdown table: title, type, size, hours, files, conflicts
3. Which tasks can run in parallel and which have dependencies
4. Total estimated hours and sessions
5. Ask: "Approve this plan? I'll create tasks in track on confirmation."

Do NOT create tasks until the user confirms.

### 6. Create tasks on approval

Get the project prefix from CLAUDE.md's `## Taskboard` section.

For each task:
```bash
track task create --project {PREFIX} --title "{title}" --priority {priority} --estimate {size} --hours {hours} --type {type} --description "{problem statement + acceptance criteria}"
```

If tasks have dependencies:
```bash
track task link {PREFIX}-{N} --blocks {PREFIX}-{M}
```

Priority mapping:
- Blockers/critical bugs → urgent
- User-facing features, high-impact fixes → high
- Improvements, moderate features → medium
- Nice-to-haves, cleanup → low

### 7. Confirm

Output:
```
{N} tasks created in track ({PREFIX}):
- {PREFIX}-{N}: {title} ({size}, {hours}h)

Total: {sum}h ({ceil(sum/3)} sessions)
Parallel-safe: {list}
Sequential: {list with arrows}

Run /plan-sprint to schedule, or start the highest-priority task now.
```
