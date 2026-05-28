package db

import (
	"database/sql"
	"testing"
)

func TestCreateAndListTasks(t *testing.T) {
	db := testDB(t)

	// Create project first
	p, err := CreateProject(db, "TST", "Test Project", "build", "build", "", "{}", 3)
	if err != nil {
		t.Fatal(err)
	}

	// Create a task with minimal fields (NULL-heavy)
	task, err := CreateTask(db, CreateTaskOpts{
		ProjectID: p.ID,
		Title:     "Minimal task",
	})
	if err != nil {
		t.Fatalf("CreateTask minimal: %v", err)
	}
	if task.Seq != 1 {
		t.Errorf("expected seq=1, got %d", task.Seq)
	}
	if task.Status != "todo" {
		t.Errorf("expected status=todo, got %s", task.Status)
	}

	// Create a fully populated task
	full, err := CreateTask(db, CreateTaskOpts{
		ProjectID:     p.ID,
		Title:         "Full task",
		Description:   "A description",
		Priority:      "high",
		Type:          "feature",
		EstimateSize:  "M",
		EstimateHours: 4.5,
		DueDate:       "2026-06-01",
		Tags:          `["frontend","bugfix"]`,
	})
	if err != nil {
		t.Fatalf("CreateTask full: %v", err)
	}
	if full.Seq != 2 {
		t.Errorf("expected seq=2, got %d", full.Seq)
	}
	if full.Priority != "high" {
		t.Errorf("expected priority=high, got %s", full.Priority)
	}
	if full.EstimateHours != 4.5 {
		t.Errorf("expected estimate_hours=4.5, got %f", full.EstimateHours)
	}

	// List tasks — should handle NULLs in minimal task without panic
	tasks, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestTaskWithParent(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "TST", "Test Project", "build", "build", "", "{}", 3)

	parent, err := CreateTask(db, CreateTaskOpts{
		ProjectID: p.ID,
		Title:     "Feature",
		Type:      "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	child, err := CreateTask(db, CreateTaskOpts{
		ProjectID: p.ID,
		Title:     "Child task",
		ParentID:  parent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Error("child parent_id not set correctly")
	}

	// List by parent
	children, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID, ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Errorf("expected 1 child, got %d", len(children))
	}
}

func TestMoveAndCompleteTask(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "TST", "Test Project", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Move me"})

	if err := MoveTask(db, task.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}
	updated, _ := GetTask(db, task.ID)
	if updated.Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", updated.Status)
	}

	if err := CompleteTask(db, task.ID, 2.5); err != nil {
		t.Fatal(err)
	}
	done, _ := GetTask(db, task.ID)
	if done.Status != "done" {
		t.Errorf("expected done, got %s", done.Status)
	}
	if done.ActualHours != 2.5 {
		t.Errorf("expected actual_hours=2.5, got %f", done.ActualHours)
	}
	if done.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestNullFieldsInScan(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "TST", "Test Project", "build", "build", "", "{}", 3)

	// Insert directly with explicit NULLs to simulate data created without defaults
	_, err := db.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, created_at, updated_at)
		VALUES ('test-null-id', ?, 99, 'Null fields task', 'todo', 'medium', 'task', '2026-05-01T00:00:00Z', '2026-05-01T00:00:00Z')`, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	// This should NOT panic or error — all nullable columns are NULL
	task, err := GetTask(db, "test-null-id")
	if err != nil {
		t.Fatalf("GetTask with NULLs failed: %v", err)
	}
	if task.Title != "Null fields task" {
		t.Errorf("unexpected title: %s", task.Title)
	}
	if task.EstimateHours != 0 {
		t.Errorf("expected 0 for null estimate_hours, got %f", task.EstimateHours)
	}

	// Also test via ListTasks
	tasks, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID})
	if err != nil {
		t.Fatalf("ListTasks with NULL rows: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestAutoActualHours_SingleStint(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Auto time"})

	// Manually insert history to simulate 2 hours in_progress
	db.Exec(`DELETE FROM task_status_history WHERE task_id = ?`, task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'todo', '2026-01-01T08:00:00Z', '2026-01-01T09:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T09:00:00Z', '2026-01-01T11:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', '2026-01-01T11:00:00Z')`,
		NewID(), task.ID)
	// Set status to in_progress so MoveTask to done is valid
	db.Exec(`UPDATE tasks SET status = 'in_progress' WHERE id = ?`, task.ID)

	if err := MoveTask(db, task.ID, "done"); err != nil {
		t.Fatal(err)
	}

	got, _ := GetTask(db, task.ID)
	// Should be ~2 hours (the in_progress stint from 09:00 to 11:00)
	if got.ActualHours < 1.9 || got.ActualHours > 2.1 {
		t.Errorf("expected ~2.0h, got %f", got.ActualHours)
	}
}

func TestAutoActualHours_MultipleStints(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Multi stint"})

	// Clear auto-created history, insert controlled rows
	db.Exec(`DELETE FROM task_status_history WHERE task_id = ?`, task.ID)
	// Stint 1: 1 hour in_progress
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T09:00:00Z', '2026-01-01T10:00:00Z')`,
		NewID(), task.ID)
	// Blocked for 2 hours (doesn't count)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'blocked', '2026-01-01T10:00:00Z', '2026-01-01T12:00:00Z')`,
		NewID(), task.ID)
	// Stint 2: 30 min in_progress
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T12:00:00Z', '2026-01-01T12:30:00Z')`,
		NewID(), task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', '2026-01-01T12:30:00Z')`,
		NewID(), task.ID)
	db.Exec(`UPDATE tasks SET status = 'in_progress' WHERE id = ?`, task.ID)

	if err := MoveTask(db, task.ID, "done"); err != nil {
		t.Fatal(err)
	}

	got, _ := GetTask(db, task.ID)
	// Should be 1.5 hours (1h + 0.5h)
	if got.ActualHours < 1.4 || got.ActualHours > 1.6 {
		t.Errorf("expected ~1.5h, got %f", got.ActualHours)
	}
}

func TestAutoActualHours_NeverStarted(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Skip to done"})

	// Move directly from todo to done (no in_progress stint)
	if err := MoveTask(db, task.ID, "done"); err != nil {
		t.Fatal(err)
	}

	got, _ := GetTask(db, task.ID)
	if got.ActualHours != 0 {
		t.Errorf("expected 0 actual_hours for never-started task, got %f", got.ActualHours)
	}
}

func TestAutoActualHours_Reopened(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Reopen test"})

	// Set up: first stint 1h, done, then reopen with another 2h stint
	db.Exec(`DELETE FROM task_status_history WHERE task_id = ?`, task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T09:00:00Z', '2026-01-01T10:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'done', '2026-01-01T10:00:00Z', '2026-01-01T14:00:00Z')`,
		NewID(), task.ID)
	// Reopened: second stint 2h
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T14:00:00Z', '2026-01-01T16:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', '2026-01-01T16:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`UPDATE tasks SET status = 'in_progress' WHERE id = ?`, task.ID)

	if err := MoveTask(db, task.ID, "done"); err != nil {
		t.Fatal(err)
	}

	got, _ := GetTask(db, task.ID)
	// Should be 3h total (1h + 2h from both in_progress stints)
	if got.ActualHours < 2.9 || got.ActualHours > 3.1 {
		t.Errorf("expected ~3.0h, got %f", got.ActualHours)
	}
}

func TestAutoActualHours_CompleteTaskOverrides(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Override test"})

	// Set up a 1h in_progress stint
	db.Exec(`DELETE FROM task_status_history WHERE task_id = ?`, task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T09:00:00Z', '2026-01-01T10:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'in_progress', '2026-01-01T10:00:00Z')`,
		NewID(), task.ID)
	db.Exec(`UPDATE tasks SET status = 'in_progress' WHERE id = ?`, task.ID)

	// CompleteTask with explicit hours should override auto-computed
	if err := CompleteTask(db, task.ID, 5.0); err != nil {
		t.Fatal(err)
	}

	got, _ := GetTask(db, task.ID)
	if got.ActualHours != 5.0 {
		t.Errorf("expected explicit 5.0h to override, got %f", got.ActualHours)
	}
}

func TestComputeActiveHours_Isolated(t *testing.T) {
	d := testDB(t)
	p, _ := CreateProject(d, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(d, CreateTaskOpts{ProjectID: p.ID, Title: "Compute test"})

	// Insert known history
	d.Exec(`DELETE FROM task_status_history WHERE task_id = ?`, task.ID)
	d.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at, exited_at) VALUES (?, ?, 'in_progress', '2026-01-01T00:00:00Z', '2026-01-01T01:30:00Z')`,
		NewID(), task.ID)

	tx, _ := d.Begin()
	defer tx.Rollback()

	hours, err := computeActiveHours(tx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hours < 1.4 || hours > 1.6 {
		t.Errorf("expected ~1.5h, got %f", hours)
	}
}

func TestComputeActiveHours_NoHistory(t *testing.T) {
	d := testDB(t)
	tx, _ := d.Begin()
	defer tx.Rollback()

	hours, err := computeActiveHours(tx, "nonexistent-id")
	if err != nil {
		t.Fatal(err)
	}
	if hours != 0 {
		t.Errorf("expected 0 for no history, got %f", hours)
	}
}

// Silence unused import
var _ = sql.ErrNoRows
