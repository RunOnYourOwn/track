package db

import (
	"testing"
)

func TestBlockerLifecycle(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "BLK", "Blocker Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Blocked task"})

	// Create blocker with task
	b, err := CreateBlocker(db, p.ID, "Waiting on API", "external", task.ID, "alice", "2026-06-01", "needs partner response")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "Waiting on API" {
		t.Errorf("expected title 'Waiting on API', got %s", b.Title)
	}
	if b.TaskID == nil || *b.TaskID != task.ID {
		t.Error("expected task_id to be set")
	}
	if b.EscalationDate == nil || *b.EscalationDate != "2026-06-01" {
		t.Error("expected escalation_date")
	}

	// Create blocker without task
	b2, err := CreateBlocker(db, p.ID, "Env access", "internal", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if b2.TaskID != nil {
		t.Error("expected nil task_id")
	}

	// List open blockers
	open, err := ListBlockers(db, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 {
		t.Fatalf("expected 2 open blockers, got %d", len(open))
	}

	// Resolve one
	if err := ResolveBlocker(db, b.ID); err != nil {
		t.Fatal(err)
	}

	// List open again
	open2, err := ListBlockers(db, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(open2) != 1 {
		t.Errorf("expected 1 open blocker after resolve, got %d", len(open2))
	}

	// List all (including resolved)
	all, err := ListBlockers(db, p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 total blockers, got %d", len(all))
	}

	// Resolve non-existent
	err = ResolveBlocker(db, "fake-id")
	if err == nil {
		t.Error("expected error resolving non-existent blocker")
	}
}

func TestGetBlocker(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "GBK", "Get Blocker", "build", "build", "", "{}", 3)
	b, _ := CreateBlocker(db, p.ID, "Test", "technical", "", "", "", "notes here")

	got, err := GetBlocker(db, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes != "notes here" {
		t.Errorf("expected notes 'notes here', got %s", got.Notes)
	}

	_, err = GetBlocker(db, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent blocker")
	}
}
