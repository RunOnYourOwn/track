package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

type ProjectInfo struct {
	ID     string
	Prefix string
}

type TaskRecord struct {
	ID           string
	Status       string
	Title        string
	Description  string
	AgentContext string
	UpdatedAt    time.Time
}

// LoadAdoTaskIndex loads all ADO-sourced tasks for a project into a map keyed by ADO work item ID.
func LoadAdoTaskIndex(conn *sql.DB, projectID string) map[int]*TaskRecord {
	index := map[int]*TaskRecord{}

	rows, err := conn.Query(`SELECT id, status, title, description, agent_context, updated_at FROM tasks WHERE project_id = ? AND source_type = 'ado'`, projectID)
	if err != nil {
		return index
	}
	defer rows.Close()

	for rows.Next() {
		var t TaskRecord
		var updatedAt string
		if err := rows.Scan(&t.ID, &t.Status, &t.Title, &t.Description, &t.AgentContext, &updatedAt); err != nil {
			continue
		}
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		var ctx struct {
			AdoID int `json:"ado_id"`
		}
		if err := json.Unmarshal([]byte(t.AgentContext), &ctx); err != nil {
			continue
		}
		if ctx.AdoID > 0 {
			rec := t
			index[ctx.AdoID] = &rec
		}
	}
	return index
}

// BuildAdoIDIndex builds a map from ADO work item ID to local task ID for a project.
func BuildAdoIDIndex(conn *sql.DB, projectID string) map[int]string {
	index := map[int]string{}

	rows, err := conn.Query(`SELECT id, agent_context FROM tasks WHERE project_id = ? AND source_type = 'ado'`, projectID)
	if err != nil {
		return index
	}
	defer rows.Close()

	for rows.Next() {
		var id, ctxStr string
		if err := rows.Scan(&id, &ctxStr); err != nil {
			continue
		}
		var ctx struct {
			AdoID int `json:"ado_id"`
		}
		if err := json.Unmarshal([]byte(ctxStr), &ctx); err != nil {
			continue
		}
		if ctx.AdoID > 0 {
			index[ctx.AdoID] = id
		}
	}
	return index
}
