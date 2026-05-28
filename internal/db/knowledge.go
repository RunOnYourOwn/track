package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/RunOnYourOwn/track/internal/models"
)

// --- Decisions ---

type CreateDecisionOpts struct {
	ProjectID    string
	TaskID       string
	Title        string
	Context      string
	Options      string // JSON array string, e.g. `["opt a","opt b"]`
	RevisitBy    string // YYYY-MM-DD or empty
	DecidedBy    string
	SupersedesID string
}

func CreateDecision(db *sql.DB, opts CreateDecisionOpts) (*models.Decision, error) {
	id := NewID()
	now := Now()

	decidedBy := opts.DecidedBy
	if decidedBy == "" {
		decidedBy = "collaborative"
	}
	options := opts.Options
	if options == "" {
		options = "[]"
	}

	var taskID *string
	if opts.TaskID != "" {
		taskID = &opts.TaskID
	}
	var revisitBy *string
	if opts.RevisitBy != "" {
		revisitBy = &opts.RevisitBy
	}
	var supersedesID *string
	if opts.SupersedesID != "" {
		supersedesID = &opts.SupersedesID
	}

	_, err := db.Exec(`
		INSERT INTO decisions (id, project_id, task_id, title, context, options, decided_by, revisit_by, status, supersedes_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id, opts.ProjectID, taskID, opts.Title, opts.Context, options, decidedBy, revisitBy, supersedesID, now)
	if err != nil {
		return nil, fmt.Errorf("create decision: %w", err)
	}
	return GetDecision(db, id)
}

func GetDecision(db *sql.DB, id string) (*models.Decision, error) {
	row := db.QueryRow(`
		SELECT id, project_id, task_id, title, context, options, decision, rationale,
		       decided_by, decided_at, revisit_by, status, supersedes_id, created_at
		FROM decisions WHERE id = ?`, id)
	return scanDecision(row)
}

func ListDecisions(db *sql.DB, projectID string, statuses []string, expiring bool) ([]models.Decision, error) {
	query := `
		SELECT id, project_id, task_id, title, context, options, decision, rationale,
		       decided_by, decided_at, revisit_by, status, supersedes_id, created_at
		FROM decisions`

	var args []any
	var conditions []string

	if projectID != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, projectID)
	}
	if expiring {
		conditions = append(conditions, "status = 'decided' AND revisit_by <= date('now', '+7 days')")
	} else if len(statuses) > 0 {
		placeholders := strings.Repeat("?,", len(statuses))
		placeholders = placeholders[:len(placeholders)-1]
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", placeholders))
		for _, s := range statuses {
			args = append(args, s)
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decisions []models.Decision
	for rows.Next() {
		d, err := scanDecisionRows(rows)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, *d)
	}
	return decisions, rows.Err()
}

func ResolveDecision(db *sql.DB, id, decision, rationale string) error {
	now := Now()
	res, err := db.Exec(`
		UPDATE decisions SET decision = ?, rationale = ?, status = 'decided', decided_at = ? WHERE id = ?`,
		decision, rationale, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("decision %q not found", id)
	}
	return nil
}

func scanDecision(row *sql.Row) (*models.Decision, error) {
	var d models.Decision
	var taskID, decidedAt, revisitBy, supersedesID sql.NullString
	var createdAt string

	err := row.Scan(
		&d.ID, &d.ProjectID, &taskID, &d.Title, &d.Context, &d.Options,
		&d.Decision, &d.Rationale, &d.DecidedBy, &decidedAt, &revisitBy,
		&d.Status, &supersedesID, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	if taskID.Valid {
		d.TaskID = &taskID.String
	}
	if decidedAt.Valid {
		t, _ := parseTime(decidedAt.String)
		d.DecidedAt = &t
	}
	if revisitBy.Valid {
		d.RevisitBy = &revisitBy.String
	}
	if supersedesID.Valid {
		d.SupersedesID = &supersedesID.String
	}
	d.CreatedAt, _ = parseTime(createdAt)
	return &d, nil
}

func scanDecisionRows(rows *sql.Rows) (*models.Decision, error) {
	var d models.Decision
	var taskID, decidedAt, revisitBy, supersedesID sql.NullString
	var createdAt string

	err := rows.Scan(
		&d.ID, &d.ProjectID, &taskID, &d.Title, &d.Context, &d.Options,
		&d.Decision, &d.Rationale, &d.DecidedBy, &decidedAt, &revisitBy,
		&d.Status, &supersedesID, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	if taskID.Valid {
		d.TaskID = &taskID.String
	}
	if decidedAt.Valid {
		t, _ := parseTime(decidedAt.String)
		d.DecidedAt = &t
	}
	if revisitBy.Valid {
		d.RevisitBy = &revisitBy.String
	}
	if supersedesID.Valid {
		d.SupersedesID = &supersedesID.String
	}
	d.CreatedAt, _ = parseTime(createdAt)
	return &d, nil
}

// --- Learnings ---

type CreateLearningOpts struct {
	ProjectID string
	TaskID    string
	Title     string
	Body      string
	Category  string
	AppliesTo string // JSON array string, e.g. `["PROJ","ACME"]`
}

func CreateLearning(db *sql.DB, opts CreateLearningOpts) (*models.Learning, error) {
	id := NewID()
	now := Now()

	category := opts.Category
	if category == "" {
		category = "pattern"
	}
	appliesTo := opts.AppliesTo
	if appliesTo == "" {
		appliesTo = "[]"
	}

	var taskID *string
	if opts.TaskID != "" {
		taskID = &opts.TaskID
	}

	_, err := db.Exec(`
		INSERT INTO learnings (id, project_id, task_id, title, body, category, applies_to, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, opts.ProjectID, taskID, opts.Title, opts.Body, category, appliesTo, now)
	if err != nil {
		return nil, fmt.Errorf("create learning: %w", err)
	}
	return GetLearning(db, id)
}

func GetLearning(db *sql.DB, id string) (*models.Learning, error) {
	row := db.QueryRow(`
		SELECT id, project_id, task_id, title, body, category, applies_to, created_at
		FROM learnings WHERE id = ?`, id)
	return scanLearning(row)
}

func ListLearnings(db *sql.DB, projectID, category string) ([]models.Learning, error) {
	query := `
		SELECT id, project_id, task_id, title, body, category, applies_to, created_at
		FROM learnings`

	var args []any
	var conditions []string

	if projectID != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, projectID)
	}
	if category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, category)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var learnings []models.Learning
	for rows.Next() {
		l, err := scanLearningRows(rows)
		if err != nil {
			return nil, err
		}
		learnings = append(learnings, *l)
	}
	return learnings, rows.Err()
}

// SearchLearnings does a LIKE search across title and body. When projectID is
// non-empty the search is scoped to that project (empty = all projects, used by
// the MCP global-search tool). FTS5 can replace this later without API change.
func SearchLearnings(db *sql.DB, projectID, query string) ([]models.Learning, error) {
	q := `
		SELECT id, project_id, task_id, title, body, category, applies_to, created_at
		FROM learnings
		WHERE (title LIKE '%'||?||'%' OR body LIKE '%'||?||'%')`
	args := []any{query, query}
	if projectID != "" {
		q += ` AND project_id = ?`
		args = append(args, projectID)
	}
	q += ` ORDER BY created_at DESC`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var learnings []models.Learning
	for rows.Next() {
		l, err := scanLearningRows(rows)
		if err != nil {
			return nil, err
		}
		learnings = append(learnings, *l)
	}
	return learnings, rows.Err()
}

func scanLearning(row *sql.Row) (*models.Learning, error) {
	var l models.Learning
	var taskID sql.NullString
	var createdAt string

	err := row.Scan(&l.ID, &l.ProjectID, &taskID, &l.Title, &l.Body, &l.Category, &l.AppliesTo, &createdAt)
	if err != nil {
		return nil, err
	}
	if taskID.Valid {
		l.TaskID = &taskID.String
	}
	l.CreatedAt, _ = parseTime(createdAt)
	return &l, nil
}

func scanLearningRows(rows *sql.Rows) (*models.Learning, error) {
	var l models.Learning
	var taskID sql.NullString
	var createdAt string

	err := rows.Scan(&l.ID, &l.ProjectID, &taskID, &l.Title, &l.Body, &l.Category, &l.AppliesTo, &createdAt)
	if err != nil {
		return nil, err
	}
	if taskID.Valid {
		l.TaskID = &taskID.String
	}
	l.CreatedAt, _ = parseTime(createdAt)
	return &l, nil
}
