# Track

CLI + web UI for local project management. Single Go binary, SQLite database, no external dependencies.

## Development

- Language: Go 1.23+
- Database: SQLite (stored at ~/.track/track.db)
- Web UI: vanilla JS + CSS (no build step), served from embedded filesystem
- Test: `go test ./...`
- Build: `make build` → `./track` at the repo root. `make check` = vet + test + build;
  `make install` copies to `~/bin`. (No `bin/` dir and no `make release` target.)

### Running the local server (read this before debugging "the UI looks wrong")

The single binary **embeds `web/static/` (the whole frontend) AND the API routes
at compile time** (`web/embed.go`). A running `track serve` keeps serving the
exact build it was started from — editing `web/static/` or any Go file has **no
effect on a live server**. After ANY change (Go or static), you must **rebuild
AND restart**:

```bash
go build -o track .   # or: make build
# stop the running instance, then start the new one:
kill "$(cat ~/.track/track.pid)" 2>/dev/null; ./track serve
```

`track serve` daemonizes by default (PID in `~/.track/track.pid`, logs in
`~/.track/track.log`); pass `--foreground` to run inline. **Symptom of a stale
binary:** a newly added `/api/...` route returns the SPA's `index.html` instead
of JSON (the unmatched path falls through to the `GET /` catch-all), or a page
renders pre-fix behavior. When something looks inaccurate in the browser, first
confirm the server was rebuilt + restarted after the change.

## Structure
```
cmd/           — CLI commands (one file per subcommand)
internal/db/   — SQLite queries and schema
internal/models/ — shared types
web/api/       — HTTP handlers
web/static/    — JS, CSS, HTML (embedded at compile time)
web/embed.go   — go:embed directives
docs/          — workflow docs, specs
skills/        — Claude Code skill definitions (deployed to ~/.claude/skills/)
```

## Key Commands
- `track serve` — start HTTP server (default :3011)
- `track project create --prefix PREFIX --name "Name"`
- `track project edit PREFIX` — edit settings (`--name/--phase/--phase-type/--wip-limit/--task-sort`); only passed flags change
- `track project delete PREFIX` — cascade-delete a project + all its data (prompts to retype the prefix; `--yes` skips)
- `track task create/move/done/cancel/list/get/next/delete/link` (`done --note`, `cancel --reason`)
- `track session start/end/log`
- `track sprint create/add/remove/start/complete/list/tasks`
- `track status/velocity/health/snapshot` (top-level commands — there is no `track report` parent)
- `track blocker create/list/resolve`
- `track ado config/pull/status`

## Conventions
- No ORM — raw SQL with `database/sql`
- Schema: new tables/columns for a fresh DB go in the `schema` const in
  `internal/db/db.go`; changes that must apply to EXISTING databases go in the
  versioned `orderedMigrations` framework in the same file (e.g. `tasks.start_date`).
  Both run automatically when the DB is opened.
- New commands: add a file in `cmd/`; it self-registers via `init()` (not `root.go`)
- Tests: table-driven, use real SQLite via `db.OpenTestDB` (a temp file, not `:memory:`)
- Web static files: edit in `web/static/`, then rebuild AND restart the server
  to pick up changes (the binary embeds them — see "Running the local server")
- Skills stay in sync with the code: whenever you change CLI commands, flags, MCP
  tools, or workflow behavior, check every skill that references them and update
  it — in BOTH the repo's `skills/` (the source) and the deployed copies under
  `~/.claude/skills/`. Do this before committing/pushing the change, so the skills
  never drift from the behavior they invoke.

## Task Hierarchy (what track manages)
- Epic: release milestone (--type epic) — POC, MVP1, Production
- Feature: user capability (--type feature, --parent FEATURE_ID)
- Task: one session of work (--type task, --parent FEATURE_ID)

---

## Using Track with an AI Agent

This section documents how an LLM coding agent (Claude Code, etc.) should interact with track during development sessions. Copy or adapt this for your own project's CLAUDE.md.

### Workflow (skills)

Install skills from `skills/` into `~/.claude/skills/`. The typical session:

1. `/session-start` — reads your project CLAUDE.md, queries the board, checks repos
2. Pick a task (or `/plan` → `/decompose` to create tasks from a feature idea)
3. `/estimate` — size unsized tasks (T-shirt + hours)
4. `/plan-sprint` — select work for the week by capacity
5. `/parallel-sprint` — execute parallel-safe tasks in git worktrees
6. `/session-end` — marks tasks done, logs hours, writes session notes

Skills invoke track via CLI (not MCP). They expect:
- Track binary on PATH
- A project created: `track project create --prefix PREFIX --name "Name"`
- `## Taskboard` section in your project CLAUDE.md with the prefix

### MCP Integration

The MCP server (`track mcp`) exposes these tools over stdio JSON-RPC:

| Tool | Purpose |
|------|---------|
| `track_project_list` | List all projects |
| `track_project_create` | Create a project (prefix + name required; optional phase/phase_type/wip_limit) |
| `track_project_update` | Edit project settings (name/phase/phase_type/wip_limit/task_sort) |
| `track_project_delete` | Delete a project + all its data (cascade; requires `confirm` = prefix) |
| `track_task_list` | List tasks (filter by project/status/priority) |
| `track_task_create` | Create a task with title, type, priority, estimate |
| `track_task_get` | Get full task details by ID |
| `track_task_move` | Move task to status (todo/in_progress/blocked/done) |
| `track_task_done` | Mark done (optional actual hours + completion note) |
| `track_task_cancel` | Cancel a task (terminal) with an optional reason |
| `track_task_next` | Suggest highest-priority unblocked task |
| `track_task_link` | Create dependency (from blocks to) |
| `track_task_unlink` | Remove dependency (from blocks to) |
| `track_task_update` | Edit task fields (title, priority, start_date, due_date, parent, …) |
| `track_task_delete` | Delete a task |
| `track_session_start` | Start a dev session |
| `track_session_end` | End session with summary |
| `track_session_log` | Log time against a task |
| `track_session_current` | Get current active session |
| `track_decision_create` | Record a decision |
| `track_decision_resolve` | Resolve a pending decision |
| `track_decision_update` | Edit a decision (title/context/options/revisit_by/decided_by) |
| `track_decision_list` | List decisions (filter by project/status/expiring) |
| `track_learn` | Capture a learning |
| `track_learn_search` | Search learnings |
| `track_learn_list` | List learnings (filter by project/category) |
| `track_learn_update` | Edit a learning (title/body/category/applies_to) |
| `track_status` | Project status summary |
| `track_blocker_list` | List active blockers |
| `track_blocker_create` | Create a blocker |
| `track_blocker_resolve` | Resolve a blocker by id |
| `track_sprint_create` | Create a sprint |
| `track_sprint_list` | List a project's sprints |
| `track_sprint_start` / `track_sprint_complete` | Set sprint status active/completed |
| `track_sprint_add` / `track_sprint_remove` | Add/remove a task to/from a sprint |
| `track_sprint_tasks` | List the tasks in a sprint |

Configure in Claude Code settings:

```json
{
  "mcpServers": {
    "track": {
      "command": "track",
      "args": ["mcp"]
    }
  }
}
```

### Knowledge Capture

Capture decisions and learnings as you work — don't wait for session-end:

```bash
track decision create --project PREFIX --title "Use SQLite over Postgres" \
  --context "Single user, no concurrency needs" --decided-by collaborative

track learn create --project PREFIX --title "SQLite WAL mode needed for concurrent reads" \
  --body "Without WAL, the web UI blocks on writes" --category gotcha
```

### Session Notes Convention

Skills persist session state to `docs/session-notes/current.md`:

```
Date: YYYY-MM-DD
Branch: feature/xyz
Phase: Build — brief status
Next: 1. ..., 2. ..., 3. ...
```

This lets `/session-start` orient instantly without reading the full board.
