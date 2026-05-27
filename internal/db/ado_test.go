package db

import (
	"encoding/json"
	"testing"
)

func TestLoadAdoTaskIndex(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "ADO", "ADO Test", "build", "build", "", "{}", 3)

	// Insert tasks with source_type='ado' and agent_context containing ado_id
	ctx1, _ := json.Marshal(map[string]any{"ado_id": 101, "rev": 5})
	ctx2, _ := json.Marshal(map[string]any{"ado_id": 202, "rev": 3})
	ctxNoID, _ := json.Marshal(map[string]any{"other": "field"})

	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 1, 'ADO Task 1', 'todo', 'medium', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-01T10:00:00Z')`,
		"ado-1", p.ID, string(ctx1))
	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 2, 'ADO Task 2', 'in_progress', 'high', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-02T10:00:00Z')`,
		"ado-2", p.ID, string(ctx2))
	// Task with no ado_id in context — should be ignored
	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 3, 'ADO NoID', 'todo', 'medium', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z')`,
		"ado-3", p.ID, string(ctxNoID))
	// Non-ADO task — should be ignored
	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 4, 'Manual Task', 'todo', 'medium', 'task', '', '{}', '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z')`,
		"manual-1", p.ID)

	index := LoadAdoTaskIndex(db, p.ID)

	if len(index) != 2 {
		t.Fatalf("expected 2 entries in index, got %d", len(index))
	}

	rec1, ok := index[101]
	if !ok {
		t.Fatal("expected ado_id 101 in index")
	}
	if rec1.ID != "ado-1" {
		t.Errorf("expected ID 'ado-1', got %s", rec1.ID)
	}
	if rec1.Status != "todo" {
		t.Errorf("expected status 'todo', got %s", rec1.Status)
	}

	rec2, ok := index[202]
	if !ok {
		t.Fatal("expected ado_id 202 in index")
	}
	if rec2.ID != "ado-2" {
		t.Errorf("expected ID 'ado-2', got %s", rec2.ID)
	}
	if rec2.Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got %s", rec2.Status)
	}
}

func TestLoadAdoTaskIndexEmpty(t *testing.T) {
	db := testDB(t)

	// Non-existent project returns empty map, no error
	index := LoadAdoTaskIndex(db, "nonexistent-project")
	if len(index) != 0 {
		t.Errorf("expected empty index, got %d entries", len(index))
	}
}

func TestBuildAdoIDIndex(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "AID", "ADO ID Index", "build", "build", "", "{}", 3)

	ctx1, _ := json.Marshal(map[string]any{"ado_id": 301})
	ctx2, _ := json.Marshal(map[string]any{"ado_id": 402})
	ctxBad := `not valid json`

	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 1, 'T1', 'todo', 'medium', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z')`,
		"id-1", p.ID, string(ctx1))
	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 2, 'T2', 'done', 'high', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z')`,
		"id-2", p.ID, string(ctx2))
	// Bad JSON — should be skipped
	db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 3, 'T3', 'todo', 'low', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z')`,
		"id-3", p.ID, ctxBad)

	index := BuildAdoIDIndex(db, p.ID)

	if len(index) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(index))
	}
	if index[301] != "id-1" {
		t.Errorf("expected id-1 for ado_id 301, got %s", index[301])
	}
	if index[402] != "id-2" {
		t.Errorf("expected id-2 for ado_id 402, got %s", index[402])
	}
}

func TestBuildAdoIDIndexEmpty(t *testing.T) {
	db := testDB(t)

	index := BuildAdoIDIndex(db, "nonexistent")
	if len(index) != 0 {
		t.Errorf("expected empty index, got %d", len(index))
	}
}
