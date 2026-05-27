package db

import (
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
