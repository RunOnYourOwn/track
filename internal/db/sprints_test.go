package db

import (
	"testing"
)

func TestSprintLifecycle(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "SPR", "Sprint Test", "build", "build", "", "{}", 3)

	// Create sprint with all fields
	s, err := CreateSprint(db, CreateSprintOpts{
		ProjectID: p.ID,
		Name:      "Sprint 1",
		Goal:      "Ship the feature",
		StartDate: "2026-06-01",
		EndDate:   "2026-06-14",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Sprint 1" {
		t.Errorf("expected name 'Sprint 1', got %s", s.Name)
	}
	if s.Status != "planned" {
		t.Errorf("expected status planned, got %s", s.Status)
	}
	if s.StartDate == nil || *s.StartDate != "2026-06-01" {
		t.Error("expected start_date 2026-06-01")
	}

	// Create sprint with minimal fields
	s2, err := CreateSprint(db, CreateSprintOpts{
		ProjectID: p.ID,
		Name:      "Sprint 2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s2.StartDate != nil {
		t.Error("expected nil start_date for minimal sprint")
	}

	// List sprints
	sprints, err := ListSprints(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sprints) != 2 {
		t.Fatalf("expected 2 sprints, got %d", len(sprints))
	}

	// Get sprint
	got, err := GetSprint(db, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Goal != "Ship the feature" {
		t.Errorf("expected goal, got %s", got.Goal)
	}

	// Update status
	if err := UpdateSprintStatus(db, s.ID, "active"); err != nil {
		t.Fatal(err)
	}
	active, _ := GetSprint(db, s.ID)
	if active.Status != "active" {
		t.Errorf("expected active, got %s", active.Status)
	}
}

func TestSprintTasks(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "SPT", "Sprint Tasks", "build", "build", "", "{}", 3)
	sprint, _ := CreateSprint(db, CreateSprintOpts{ProjectID: p.ID, Name: "S1"})
	t1, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})
	t2, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 2"})

	// Add tasks
	if err := AddTaskToSprint(db, sprint.ID, t1.ID); err != nil {
		t.Fatal(err)
	}
	if err := AddTaskToSprint(db, sprint.ID, t2.ID); err != nil {
		t.Fatal(err)
	}

	// Duplicate add should be no-op (INSERT OR IGNORE)
	if err := AddTaskToSprint(db, sprint.ID, t1.ID); err != nil {
		t.Fatal(err)
	}

	// List sprint tasks
	tasks, err := ListSprintTasks(db, sprint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 sprint tasks, got %d", len(tasks))
	}

	// Remove task
	if err := RemoveTaskFromSprint(db, sprint.ID, t1.ID); err != nil {
		t.Fatal(err)
	}
	tasks2, _ := ListSprintTasks(db, sprint.ID)
	if len(tasks2) != 1 {
		t.Errorf("expected 1 task after remove, got %d", len(tasks2))
	}
}
