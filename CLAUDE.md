# Track

CLI + web UI for local project management. Single Go binary, SQLite database, no external dependencies.

## Development

- Language: Go 1.23+
- Database: SQLite (stored at ~/.track/track.db)
- Web UI: vanilla JS + CSS (no build step), served from embedded filesystem
- Test: `go test ./...`
- Build: `make build` (outputs to bin/)
- Cross-compile: `make release` (darwin-arm64, windows-amd64)

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
- `track project create PREFIX "Name"`
- `track task create/move/done/list/get/next/delete/link`
- `track session start/end/log`
- `track sprint create/add/remove/start/complete/list/tasks`
- `track report status/velocity/health/snapshot`
- `track blocker create/list/resolve`
- `track ado config/pull/status`

## Conventions
- No ORM — raw SQL with `database/sql`
- Schema changes: add to the `schema` const in `internal/db/db.go` (auto-migrates on serve)
- New commands: add file in `cmd/`, register in `cmd/root.go`
- Tests: table-driven, use real SQLite (in-memory `:memory:`)
- Web static files: edit in `web/static/`, recompile to pick up changes
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
- A project created: `track project create PREFIX "Name"`
- `## Taskboard` section in your project CLAUDE.md with the prefix

### MCP Integration

The MCP server (`track mcp`) exposes these tools over stdio JSON-RPC:

| Tool | Purpose |
|------|---------|
| `track_project_list` | List all projects |
| `track_task_list` | List tasks (filter by project/status/priority) |
| `track_task_create` | Create a task with title, type, priority, estimate |
| `track_task_get` | Get full task details by ID |
| `track_task_move` | Move task to status (todo/in_progress/blocked/done) |
| `track_task_done` | Mark done with optional actual hours |
| `track_task_next` | Suggest highest-priority unblocked task |
| `track_task_link` | Create dependency (from blocks to) |
| `track_session_start` | Start a dev session |
| `track_session_end` | End session with summary |
| `track_session_log` | Log time against a task |
| `track_session_current` | Get current active session |
| `track_decision_create` | Record a decision |
| `track_decision_resolve` | Resolve a pending decision |
| `track_learn` | Capture a learning |
| `track_learn_search` | Search learnings |
| `track_status` | Project status summary |
| `track_blocker_list` | List active blockers |

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

track learning create --project PREFIX --title "SQLite WAL mode needed for concurrent reads" \
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
