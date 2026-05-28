package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	instance *sql.DB
	initErr  error
	initDone bool
	mu       sync.Mutex
)

func DBPath() string {
	if p := os.Getenv("TRACK_DB"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".track", "track.db")
}

func Open() (*sql.DB, error) {
	mu.Lock()
	defer mu.Unlock()

	if initDone {
		return instance, initErr
	}

	path := DBPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		initErr = fmt.Errorf("create db dir: %w", err)
		initDone = true
		return nil, initErr
	}

	d, err := sql.Open("sqlite", path)
	if err != nil {
		initErr = err
		initDone = true
		return nil, initErr
	}
	d.SetMaxOpenConns(1)

	// Restrict DB file permissions
	if err := os.Chmod(path, 0600); err != nil && !os.IsNotExist(err) {
		d.Close()
		initErr = fmt.Errorf("chmod db: %w", err)
		initDone = true
		return nil, initErr
	}

	if err = configurePragmas(d); err != nil {
		d.Close()
		return nil, err
	}
	if err = migrate(d); err != nil {
		d.Close()
		return nil, err
	}

	instance = d
	initDone = true
	return instance, nil
}

func Close() error {
	if instance == nil {
		return nil
	}
	_, _ = instance.Exec("PRAGMA optimize")
	return instance.Close()
}

func configurePragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -32000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 134217728",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	return nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}
	// Add columns that may not exist in older databases
	migrations := []string{
		`ALTER TABLE tasks ADD COLUMN type TEXT NOT NULL DEFAULT 'task'`,
		`ALTER TABLE tasks ADD COLUMN estimate_agent_minutes INTEGER DEFAULT 0`,
		`ALTER TABLE projects ADD COLUMN external_id TEXT DEFAULT ''`,
	}
	for _, m := range migrations {
		_, _ = db.Exec(m) // ignore "duplicate column" errors
	}
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    prefix      TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    phase       TEXT DEFAULT '',
    phase_type  TEXT DEFAULT 'build',
    external_id TEXT DEFAULT '',
    metadata    TEXT DEFAULT '{}',
    wip_limit   INTEGER DEFAULT 3,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    seq             INTEGER NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'todo',
    priority        TEXT NOT NULL DEFAULT 'medium',
    estimate_size   TEXT DEFAULT '',
    estimate_hours  REAL DEFAULT 0,
    estimate_agent_minutes INTEGER DEFAULT 0,
    actual_hours    REAL DEFAULT 0,
    type            TEXT NOT NULL DEFAULT 'task',
    parent_id       TEXT REFERENCES tasks(id),
    sort_order      INTEGER DEFAULT 0,
    source_type     TEXT DEFAULT 'planned',
    agent_context   TEXT DEFAULT '{}',
    tags            TEXT DEFAULT '[]',
    due_date        TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    completed_at    TEXT,
    is_rework       INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS dependencies (
    from_task_id TEXT NOT NULL REFERENCES tasks(id),
    to_task_id   TEXT NOT NULL REFERENCES tasks(id),
    dep_type     TEXT NOT NULL DEFAULT 'blocks',
    reason       TEXT DEFAULT '',
    PRIMARY KEY (from_task_id, to_task_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    branch      TEXT DEFAULT '',
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    summary     TEXT DEFAULT '',
    tasks_json  TEXT DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS time_entries (
    id              TEXT PRIMARY KEY,
    task_id         TEXT NOT NULL REFERENCES tasks(id),
    session_id      TEXT REFERENCES sessions(id),
    hours           REAL DEFAULT 0,
    note            TEXT DEFAULT '',
    created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_status_history (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL REFERENCES tasks(id),
    status      TEXT NOT NULL,
    entered_at  TEXT NOT NULL,
    exited_at   TEXT
);

CREATE TABLE IF NOT EXISTS task_commits (
    task_id        TEXT NOT NULL REFERENCES tasks(id),
    commit_hash    TEXT NOT NULL,
    repo           TEXT NOT NULL,
    committed_at   TEXT NOT NULL,
    message        TEXT DEFAULT '',
    files_changed  TEXT DEFAULT '[]',
    PRIMARY KEY (task_id, commit_hash)
);

CREATE TABLE IF NOT EXISTS deploys (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id),
    environment    TEXT DEFAULT 'production',
    deployed_at    TEXT NOT NULL,
    commit_hash    TEXT NOT NULL,
    tag            TEXT DEFAULT '',
    task_ids       TEXT DEFAULT '[]',
    triggered_by   TEXT DEFAULT 'human'
);

CREATE TABLE IF NOT EXISTS decisions (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    task_id         TEXT REFERENCES tasks(id),
    title           TEXT NOT NULL,
    context         TEXT DEFAULT '',
    options         TEXT DEFAULT '[]',
    decision        TEXT DEFAULT '',
    rationale       TEXT DEFAULT '',
    decided_by      TEXT DEFAULT 'collaborative',
    decided_at      TEXT,
    revisit_by      TEXT,
    status          TEXT DEFAULT 'open',
    supersedes_id   TEXT REFERENCES decisions(id),
    created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS learnings (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    task_id         TEXT REFERENCES tasks(id),
    title           TEXT NOT NULL,
    body            TEXT DEFAULT '',
    category        TEXT DEFAULT 'pattern',
    applies_to      TEXT DEFAULT '[]',
    created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cross_project_deps (
    id                TEXT PRIMARY KEY,
    source_project_id TEXT NOT NULL REFERENCES projects(id),
    source_task_id    TEXT REFERENCES tasks(id),
    target_project_id TEXT NOT NULL REFERENCES projects(id),
    target_task_id    TEXT REFERENCES tasks(id),
    dep_type          TEXT NOT NULL,
    notes             TEXT DEFAULT '',
    created_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS blockers (
    id              TEXT PRIMARY KEY,
    task_id         TEXT REFERENCES tasks(id),
    project_id      TEXT NOT NULL REFERENCES projects(id),
    title           TEXT NOT NULL,
    blocker_type    TEXT NOT NULL,
    owner           TEXT DEFAULT '',
    opened_at       TEXT NOT NULL,
    resolved_at     TEXT,
    escalation_date TEXT,
    notes           TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sprints (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    goal        TEXT DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'planned',
    start_date  TEXT,
    end_date    TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sprint_tasks (
    sprint_id TEXT NOT NULL REFERENCES sprints(id),
    task_id   TEXT NOT NULL REFERENCES tasks(id),
    PRIMARY KEY (sprint_id, task_id)
);

CREATE TABLE IF NOT EXISTS snapshots (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    taken_at        TEXT NOT NULL,
    total           INTEGER DEFAULT 0,
    done            INTEGER DEFAULT 0,
    in_progress     INTEGER DEFAULT 0,
    todo            INTEGER DEFAULT 0,
    blocked         INTEGER DEFAULT 0,
    hours_done      REAL DEFAULT 0,
    hours_remaining REAL DEFAULT 0,
    flow_efficiency REAL DEFAULT 0,
    rework_rate     REAL DEFAULT 0,
    health_score    REAL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_tasks_project_status ON tasks(project_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(due_date) WHERE due_date IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_project_seq ON tasks(project_id, seq);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_project_date ON snapshots(project_id, taken_at);
CREATE INDEX IF NOT EXISTS idx_dependencies_to ON dependencies(to_task_id);
CREATE INDEX IF NOT EXISTS idx_status_history_task ON task_status_history(task_id, entered_at);
CREATE INDEX IF NOT EXISTS idx_task_commits_task ON task_commits(task_id);
CREATE INDEX IF NOT EXISTS idx_task_commits_hash ON task_commits(commit_hash);
CREATE INDEX IF NOT EXISTS idx_cross_deps_target ON cross_project_deps(target_project_id);
CREATE INDEX IF NOT EXISTS idx_blockers_project ON blockers(project_id, resolved_at);
CREATE INDEX IF NOT EXISTS idx_decisions_revisit ON decisions(revisit_by) WHERE status = 'decided';
CREATE INDEX IF NOT EXISTS idx_sprints_project ON sprints(project_id, status);
CREATE INDEX IF NOT EXISTS idx_sprint_tasks_task ON sprint_tasks(task_id);
CREATE INDEX IF NOT EXISTS idx_time_entries_session ON time_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_task_commits_time ON task_commits(committed_at);
CREATE INDEX IF NOT EXISTS idx_status_history_time ON task_status_history(entered_at);
`
