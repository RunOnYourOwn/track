package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

// ResolveSprintID accepts either a full 26-char ULID or a unique id prefix (the
// CLI displays the leading 8 chars), and returns the full sprint id.
func ResolveSprintID(d *sql.DB, idOrPrefix string) (string, error) {
	if len(idOrPrefix) == 26 {
		return idOrPrefix, nil
	}
	rows, err := d.Query(`SELECT id FROM sprints WHERE id LIKE ? || '%' LIMIT 2`, idOrPrefix)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("sprint %q not found", idOrPrefix)
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("ambiguous sprint id %q (matches multiple)", idOrPrefix)
	}
}


type CreateSprintOpts struct {
	ProjectID string
	Name      string
	Goal      string
	StartDate string
	EndDate   string
}

func CreateSprint(d *sql.DB, opts CreateSprintOpts) (*models.Sprint, error) {
	id := NewID()
	now := time.Now().UTC().Format(time.RFC3339)

	var startDate, endDate *string
	if opts.StartDate != "" {
		startDate = &opts.StartDate
	}
	if opts.EndDate != "" {
		endDate = &opts.EndDate
	}

	_, err := d.Exec(`
		INSERT INTO sprints (id, project_id, name, goal, status, start_date, end_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'planned', ?, ?, ?, ?)`,
		id, opts.ProjectID, opts.Name, opts.Goal, startDate, endDate, now, now,
	)
	if err != nil {
		return nil, err
	}

	return GetSprint(d, id)
}

func GetSprint(d *sql.DB, id string) (*models.Sprint, error) {
	row := d.QueryRow(`SELECT id, project_id, name, goal, status, start_date, end_date, created_at, updated_at FROM sprints WHERE id = ?`, id)
	return scanSprint(row)
}

func ListSprints(d *sql.DB, projectID string) ([]models.Sprint, error) {
	rows, err := d.Query(`
		SELECT id, project_id, name, goal, status, start_date, end_date, created_at, updated_at
		FROM sprints WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Sprint
	for rows.Next() {
		s, err := scanSprintRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}
	return result, rows.Err()
}

// ValidSprintStatuses are the sprint lifecycle states.
var ValidSprintStatuses = map[string]bool{"planned": true, "active": true, "completed": true}

func UpdateSprintStatus(d *sql.DB, id, status string) error {
	if !ValidSprintStatuses[status] {
		return fmt.Errorf("invalid sprint status %q (expected: planned, active, completed)", status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := d.Exec(`UPDATE sprints SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("sprint %q not found", id)
	}
	return nil
}

func AddTaskToSprint(d *sql.DB, sprintID, taskID string) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO sprint_tasks (sprint_id, task_id) VALUES (?, ?)`, sprintID, taskID)
	return err
}

func RemoveTaskFromSprint(d *sql.DB, sprintID, taskID string) error {
	_, err := d.Exec(`DELETE FROM sprint_tasks WHERE sprint_id = ? AND task_id = ?`, sprintID, taskID)
	return err
}

func ListSprintTasks(d *sql.DB, sprintID string) ([]models.Task, error) {
	rows, err := d.Query(taskSelect+` JOIN sprint_tasks st ON st.task_id = t.id WHERE st.sprint_id = ? ORDER BY t.seq`, sprintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	return tasks, rows.Err()
}

func scanSprint(row *sql.Row) (*models.Sprint, error) {
	var s models.Sprint
	var createdAt, updatedAt string
	err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Goal, &s.Status, &s.StartDate, &s.EndDate, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &s, nil
}

func scanSprintRow(rows *sql.Rows) (*models.Sprint, error) {
	var s models.Sprint
	var createdAt, updatedAt string
	err := rows.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Goal, &s.Status, &s.StartDate, &s.EndDate, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &s, nil
}
