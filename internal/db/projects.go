package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/RunOnYourOwn/track/internal/models"
)

func CreateProject(db *sql.DB, prefix, name, phase, phaseType, externalID, metadata string, wipLimit int) (*models.Project, error) {
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

	_, err := db.Exec(`INSERT INTO projects (id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.ToUpper(prefix), name, phase, phaseType, externalID, metadata, wipLimit, now, now)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	return GetProjectByID(db, id)
}

func GetProjectByID(db *sql.DB, id string) (*models.Project, error) {
	row := db.QueryRow(`SELECT id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, created_at, updated_at FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func GetProjectByPrefix(db *sql.DB, prefix string) (*models.Project, error) {
	row := db.QueryRow(`SELECT id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, created_at, updated_at FROM projects WHERE prefix = ?`, strings.ToUpper(prefix))
	return scanProject(row)
}

func ListProjects(db *sql.DB) ([]models.Project, error) {
	rows, err := db.Query(`SELECT id, prefix, name, phase, phase_type, external_id, metadata, wip_limit, created_at, updated_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		p, err := scanProjectRows(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, *p)
	}
	return projects, rows.Err()
}

var allowedProjectFields = map[string]bool{
	"name": true, "phase": true, "phase_type": true,
	"external_id": true, "metadata": true, "wip_limit": true,
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
	_, err := db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (*models.Project, error) {
	var p models.Project
	var createdAt, updatedAt string
	err := row.Scan(&p.ID, &p.Prefix, &p.Name, &p.Phase, &p.PhaseType, &p.ExternalID, &p.Metadata, &p.WIPLimit, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = parseTime(createdAt)
	p.UpdatedAt, _ = parseTime(updatedAt)
	return &p, nil
}

func scanProjectRows(rows *sql.Rows) (*models.Project, error) {
	var p models.Project
	var createdAt, updatedAt string
	err := rows.Scan(&p.ID, &p.Prefix, &p.Name, &p.Phase, &p.PhaseType, &p.ExternalID, &p.Metadata, &p.WIPLimit, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = parseTime(createdAt)
	p.UpdatedAt, _ = parseTime(updatedAt)
	return &p, nil
}
