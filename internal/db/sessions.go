package db

import (
	"database/sql"
	"fmt"

	"github.com/RunOnYourOwn/track/internal/models"
)

func StartSession(db *sql.DB, projectID, branch string) (*models.Session, error) {
	id := NewID()
	now := Now()

	_, err := db.Exec(`INSERT INTO sessions (id, project_id, branch, started_at) VALUES (?, ?, ?, ?)`,
		id, projectID, branch, now)
	if err != nil {
		return nil, fmt.Errorf("start session: %w", err)
	}
	return GetSession(db, id)
}

func EndSession(db *sql.DB, id, summary string) error {
	now := Now()
	_, err := db.Exec(`UPDATE sessions SET ended_at = ?, summary = ? WHERE id = ?`, now, summary, id)
	return err
}

func GetSession(db *sql.DB, id string) (*models.Session, error) {
	row := db.QueryRow(`SELECT id, project_id, branch, started_at, ended_at, summary, tasks_json FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func GetCurrentSession(db *sql.DB, projectID string) (*models.Session, error) {
	var query string
	var args []any
	if projectID != "" {
		query = `SELECT id, project_id, branch, started_at, ended_at, summary, tasks_json FROM sessions WHERE project_id = ? AND ended_at IS NULL ORDER BY started_at DESC LIMIT 1`
		args = []any{projectID}
	} else {
		query = `SELECT id, project_id, branch, started_at, ended_at, summary, tasks_json FROM sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1`
	}
	row := db.QueryRow(query, args...)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func ListSessions(db *sql.DB, projectID string, limit int) ([]models.Session, error) {
	if limit <= 0 {
		limit = 10
	}
	var query string
	var args []any
	if projectID != "" {
		query = `SELECT id, project_id, branch, started_at, ended_at, summary, tasks_json FROM sessions WHERE project_id = ? ORDER BY started_at DESC LIMIT ?`
		args = []any{projectID, limit}
	} else {
		query = `SELECT id, project_id, branch, started_at, ended_at, summary, tasks_json FROM sessions ORDER BY started_at DESC LIMIT ?`
		args = []any{limit}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		s, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *s)
	}
	return sessions, rows.Err()
}

func LogTime(d *sql.DB, taskID, sessionID string, hours float64, note string) error {
	if hours <= 0 {
		return fmt.Errorf("hours must be positive, got %g", hours)
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	id := NewID()
	now := Now()
	var sid *string
	if sessionID != "" {
		sid = &sessionID
	}
	if _, err := tx.Exec(`INSERT INTO time_entries (id, task_id, session_id, hours, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, taskID, sid, hours, note, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET actual_hours = actual_hours + ?, updated_at = ? WHERE id = ?`, hours, now, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

func scanSession(row *sql.Row) (*models.Session, error) {
	var s models.Session
	var startedAt string
	var endedAt sql.NullString
	err := row.Scan(&s.ID, &s.ProjectID, &s.Branch, &startedAt, &endedAt, &s.Summary, &s.TasksJSON)
	if err != nil {
		return nil, err
	}
	s.StartedAt, _ = parseTime(startedAt)
	if endedAt.Valid {
		t, _ := parseTime(endedAt.String)
		s.EndedAt = &t
	}
	return &s, nil
}

func scanSessionRows(rows *sql.Rows) (*models.Session, error) {
	var s models.Session
	var startedAt string
	var endedAt sql.NullString
	err := rows.Scan(&s.ID, &s.ProjectID, &s.Branch, &startedAt, &endedAt, &s.Summary, &s.TasksJSON)
	if err != nil {
		return nil, err
	}
	s.StartedAt, _ = parseTime(startedAt)
	if endedAt.Valid {
		t, _ := parseTime(endedAt.String)
		s.EndedAt = &t
	}
	return &s, nil
}
