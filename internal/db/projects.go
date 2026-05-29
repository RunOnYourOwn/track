package db

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/RunOnYourOwn/track/internal/models"
)

// ValidationError marks an error caused by invalid caller input (as opposed to a
// server/storage failure), so HTTP handlers can map it to 400 rather than 500.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func validationErrf(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// Prefixes appear unescaped in display IDs (PREFIX-123) across every web view and
// in API URL paths, so they are constrained to a safe charset at the create
// boundary rather than relied on to be escaped at every output site.
var prefixRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]*$`)

func normalizePrefix(prefix string) (string, error) {
	p := strings.ToUpper(strings.TrimSpace(prefix))
	if p == "" {
		return "", validationErrf("project prefix is required")
	}
	if len(p) > 16 {
		return "", validationErrf("project prefix too long (max 16 chars): %q", prefix)
	}
	if !prefixRe.MatchString(p) {
		return "", validationErrf("invalid project prefix %q: use letters, digits, '-' or '_' (must start alphanumeric)", prefix)
	}
	return p, nil
}

func CreateProject(db *sql.DB, prefix, name, phase, phaseType, externalID, metadata string, wipLimit int) (*models.Project, error) {
	normPrefix, err := normalizePrefix(prefix)
	if err != nil {
		return nil, err
	}
	id := NewID()
	now := Now()
	if phaseType == "" {
		phaseType = "build"
	}
	if metadata == "" {
		metadata = "{}"
	}
	if wipLimit == 0 {
		wipLimit = 3
	}

	_, err = db.Exec(`INSERT INTO projects (id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, normPrefix, name, phase, phaseType, externalID, metadata, wipLimit, now, now)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	return GetProjectByID(db, id)
}

func GetProjectByID(db *sql.DB, id string) (*models.Project, error) {
	row := db.QueryRow(`SELECT id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, task_sort, created_at, updated_at FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func GetProjectByPrefix(db *sql.DB, prefix string) (*models.Project, error) {
	row := db.QueryRow(`SELECT id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, task_sort, created_at, updated_at FROM projects WHERE prefix = ?`, strings.ToUpper(prefix))
	return scanProject(row)
}

func ListProjects(db *sql.DB) ([]models.Project, error) {
	rows, err := db.Query(`SELECT id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, task_sort, created_at, updated_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, *p)
	}
	return projects, rows.Err()
}

var allowedProjectFields = map[string]bool{
	"name": true, "phase": true, "phase_type": true,
	"external_id": true, "metadata": true, "wip_limit": true, "task_sort": true,
}

// ValidTaskSorts are the per-project task ordering modes the board/lists honor.
// The actual ORDER BY for each lives in taskOrderBy (internal/db/tasks.go), the
// single server-side source of truth for sort order.
var ValidTaskSorts = map[string]bool{
	"priority": true, // priority, then manual order, then age (default)
	"manual":   true, // manual drag order, then priority, then age
	"created":  true, // creation order (oldest first)
	"due":      true, // due date soonest first (no due last), then priority
}

// ValidPhaseTypes are the lifecycle phases a project can be in.
var ValidPhaseTypes = map[string]bool{
	"discovery": true, "design": true, "build": true, "stabilize": true, "maintain": true,
}

func UpdateProjectField(d *sql.DB, id, field, value string) error {
	if !allowedProjectFields[field] {
		return fmt.Errorf("UpdateProjectField: disallowed field %q", field)
	}
	now := Now()
	query := fmt.Sprintf(`UPDATE projects SET %s = ?, updated_at = ? WHERE id = ?`, field)
	_, err := d.Exec(query, value, now, id)
	return err
}

func DeleteProject(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Defer FK enforcement to COMMIT so we don't have to order deletes around
	// self-references (tasks.parent_id, decisions.supersedes_id). Without this,
	// a bare DELETE FROM projects fails (FK 787) for any non-empty project.
	if _, err := tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
		return err
	}

	taskSub := `(SELECT id FROM tasks WHERE project_id = ?)`
	steps := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM task_status_history WHERE task_id IN ` + taskSub, []any{id}},
		{`DELETE FROM task_commits WHERE task_id IN ` + taskSub, []any{id}},
		{`DELETE FROM dependencies WHERE from_task_id IN ` + taskSub + ` OR to_task_id IN ` + taskSub, []any{id, id}},
		{`DELETE FROM sprint_tasks WHERE task_id IN ` + taskSub + ` OR sprint_id IN (SELECT id FROM sprints WHERE project_id = ?)`, []any{id, id}},
		{`DELETE FROM time_entries WHERE task_id IN ` + taskSub + ` OR session_id IN (SELECT id FROM sessions WHERE project_id = ?)`, []any{id, id}},
		{`DELETE FROM cross_project_deps WHERE source_project_id = ? OR target_project_id = ? OR source_task_id IN ` + taskSub + ` OR target_task_id IN ` + taskSub, []any{id, id, id, id}},
		{`DELETE FROM decisions WHERE project_id = ?`, []any{id}},
		{`DELETE FROM learnings WHERE project_id = ?`, []any{id}},
		{`DELETE FROM blockers WHERE project_id = ?`, []any{id}},
		{`DELETE FROM deploys WHERE project_id = ?`, []any{id}},
		{`DELETE FROM snapshots WHERE project_id = ?`, []any{id}},
		{`DELETE FROM sprints WHERE project_id = ?`, []any{id}},
		{`DELETE FROM tasks WHERE project_id = ?`, []any{id}},
		{`DELETE FROM sessions WHERE project_id = ?`, []any{id}},
		{`DELETE FROM projects WHERE id = ?`, []any{id}},
	}
	for _, s := range steps {
		if _, err := tx.Exec(s.query, s.args...); err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
	}
	return tx.Commit()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (*models.Project, error) {
	var p models.Project
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Prefix, &p.Name, &p.Phase, &p.PhaseType, &p.ExternalID, &p.Metadata, &p.WIPLimit, &p.TaskSort, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = parseTime(createdAt)
	p.UpdatedAt, _ = parseTime(updatedAt)
	return &p, nil
}

