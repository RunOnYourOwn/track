package db

import (
	"database/sql"
	"fmt"

	"github.com/RunOnYourOwn/track/internal/models"
)

// CreateBlocker inserts a new blocker record and returns it.
// taskID, owner, escalationDate, and notes are all optional (pass "" to omit).
func CreateBlocker(db *sql.DB, projectID, title, blockerType string, taskID, owner, escalationDate, notes string) (*models.Blocker, error) {
	id := NewID()
	now := Now()

	var taskIDPtr *string
	if taskID != "" {
		taskIDPtr = &taskID
	}
	var escalationDatePtr *string
	if escalationDate != "" {
		escalationDatePtr = &escalationDate
	}

	_, err := db.Exec(`
		INSERT INTO blockers (id, task_id, project_id, title, blocker_type, owner, opened_at, escalation_date, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, taskIDPtr, projectID, title, blockerType, owner, now, escalationDatePtr, notes)
	if err != nil {
		return nil, fmt.Errorf("create blocker: %w", err)
	}

	return GetBlocker(db, id)
}

// ResolveBlocker marks a blocker as resolved by setting resolved_at to now.
func ResolveBlocker(db *sql.DB, id string) error {
	now := Now()
	res, err := db.Exec(`UPDATE blockers SET resolved_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("blocker %q not found", id)
	}
	return nil
}

// ListBlockers returns blockers for a project. If openOnly is true, only
// unresolved blockers are returned.
func ListBlockers(db *sql.DB, projectID string, openOnly bool) ([]models.Blocker, error) {
	query := `
		SELECT id, task_id, project_id, title, blocker_type, owner, opened_at, resolved_at, escalation_date, notes
		FROM blockers
		WHERE project_id = ?`
	if openOnly {
		query += " AND resolved_at IS NULL"
	}
	query += " ORDER BY opened_at DESC"

	rows, err := db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockers []models.Blocker
	for rows.Next() {
		b, err := scanBlocker(rows)
		if err != nil {
			return nil, err
		}
		blockers = append(blockers, *b)
	}
	return blockers, rows.Err()
}

// GetBlocker returns a single blocker by its internal ID.
func GetBlocker(db *sql.DB, id string) (*models.Blocker, error) {
	row := db.QueryRow(`
		SELECT id, task_id, project_id, title, blocker_type, owner, opened_at, resolved_at, escalation_date, notes
		FROM blockers WHERE id = ?`, id)
	return scanBlocker(row)
}

// --- scanners ---

type blockerScanner interface {
	Scan(dest ...any) error
}

func scanBlocker(s blockerScanner) (*models.Blocker, error) {
	var b models.Blocker
	var taskID, resolvedAt, escalationDate sql.NullString
	var openedAt string

	if err := s.Scan(&b.ID, &taskID, &b.ProjectID, &b.Title, &b.BlockerType, &b.Owner, &openedAt, &resolvedAt, &escalationDate, &b.Notes); err != nil {
		return nil, err
	}
	if taskID.Valid {
		b.TaskID = &taskID.String
	}
	if escalationDate.Valid {
		b.EscalationDate = &escalationDate.String
	}
	b.OpenedAt, _ = parseTime(openedAt)
	if resolvedAt.Valid {
		t, _ := parseTime(resolvedAt.String)
		b.ResolvedAt = &t
	}
	return &b, nil
}

