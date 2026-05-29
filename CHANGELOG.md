# Changelog

All notable changes to track are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). The release workflow
publishes the section for each tagged version as that GitHub Release's notes.

## [Unreleased]

## [0.1.1] - 2026-05-29

Release tooling and documentation only — the `track` binary is unchanged from
v0.1.0.

### Added
- Changelog-driven GitHub Release notes: the release workflow now publishes the
  matching `CHANGELOG.md` section as the release body. (v0.1.0's notes were
  auto-generated because the changelog landed just after that tag was cut.)
- `/release` deploy skill documenting the bump → roll changelog → tag → publish flow.

## [0.1.0] - 2026-05-29

First official release — a single Go binary (CLI + embedded web UI, SQLite, no
external services) for managing AI-assisted development.

### Added
- Task hierarchy (epic → feature → task) with priorities, dependencies, and a
  status/priority vocabulary served from the backend via `GET /api/meta` (one
  source of truth for CLI, MCP, and UI).
- CLI commands for projects, tasks, sprints, sessions, blockers, and reports, plus
  `track --version`.
- MCP stdio server exposing task / project / sprint / session / decision /
  learning / blocker tools to coding agents.
- Web UI (no build step): kanban board, hierarchy tree, dependency graph with a
  weighted critical path, session timeline, and cross-project insights.
- Azure DevOps sync — pull work items, push status changes.
- Server-side rollups: epic/feature start/due dates **and** estimates derived from
  descendant tasks (read-only on parents).
- Estimation accuracy measured on the agent axis (agent estimate vs actual active
  time), surfaced in `track health`, the insights dashboard, and `track velocity`.
- `go install` / `make install` into `~/bin`, and a tag-driven release workflow
  that cross-compiles linux + darwin (amd64/arm64) and publishes binaries +
  checksums.

[Unreleased]: https://github.com/RunOnYourOwn/track/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/RunOnYourOwn/track/releases/tag/v0.1.1
[0.1.0]: https://github.com/RunOnYourOwn/track/releases/tag/v0.1.0
