package db

import (
	"testing"
)

func TestDecisionLifecycle(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "KNW", "Knowledge Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Related task"})

	// Create decision with all fields
	d, err := CreateDecision(db, CreateDecisionOpts{
		ProjectID: p.ID,
		TaskID:    task.ID,
		Title:     "Choose DB",
		Context:   "Need a database for the project",
		Options:   `["postgres","sqlite","mysql"]`,
		RevisitBy: "2026-07-01",
		DecidedBy: "team",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Choose DB" {
		t.Errorf("expected title 'Choose DB', got %s", d.Title)
	}
	if d.Status != "open" {
		t.Errorf("expected status open, got %s", d.Status)
	}
	if d.TaskID == nil || *d.TaskID != task.ID {
		t.Error("expected task_id")
	}
	if d.RevisitBy == nil || *d.RevisitBy != "2026-07-01" {
		t.Error("expected revisit_by")
	}

	// Create minimal decision
	d2, err := CreateDecision(db, CreateDecisionOpts{
		ProjectID: p.ID,
		Title:     "Minimal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d2.DecidedBy != "collaborative" {
		t.Errorf("expected default decided_by, got %s", d2.DecidedBy)
	}

	// Get decision
	got, err := GetDecision(db, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Context != "Need a database for the project" {
		t.Errorf("unexpected context: %s", got.Context)
	}

	// List decisions
	all, err := ListDecisions(db, p.ID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(all))
	}

	// List with status filter
	open, err := ListDecisions(db, p.ID, []string{"open"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 {
		t.Errorf("expected 2 open decisions, got %d", len(open))
	}

	// Resolve
	if err := ResolveDecision(db, d.ID, "sqlite", "simplest option"); err != nil {
		t.Fatal(err)
	}
	resolved, _ := GetDecision(db, d.ID)
	if resolved.Status != "decided" {
		t.Errorf("expected decided, got %s", resolved.Status)
	}
	if resolved.Decision != "sqlite" {
		t.Errorf("expected decision 'sqlite', got %s", resolved.Decision)
	}
	if resolved.DecidedAt == nil {
		t.Error("expected decided_at to be set")
	}

	// Resolve non-existent
	err = ResolveDecision(db, "fake-id", "x", "y")
	if err == nil {
		t.Error("expected error resolving non-existent decision")
	}

	// List expiring (need a decided decision with revisit_by in the past/near future)
	// d was resolved and has revisit_by = 2026-07-01; whether it shows depends on current date
	// Just verify no error
	_, err = ListDecisions(db, p.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLearningLifecycle(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "LRN", "Learning Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Related"})

	// Create with all fields
	l, err := CreateLearning(db, CreateLearningOpts{
		ProjectID: p.ID,
		TaskID:    task.ID,
		Title:     "SQLite WAL mode",
		Body:      "WAL mode improves concurrent reads significantly",
		Category:  "architecture",
		AppliesTo: `["PRJ","OTH"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if l.Title != "SQLite WAL mode" {
		t.Errorf("expected title, got %s", l.Title)
	}
	if l.Category != "architecture" {
		t.Errorf("expected category architecture, got %s", l.Category)
	}

	// Create minimal
	l2, err := CreateLearning(db, CreateLearningOpts{
		ProjectID: p.ID,
		Title:     "Minimal learning",
		Body:      "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if l2.Category != "pattern" {
		t.Errorf("expected default category 'pattern', got %s", l2.Category)
	}

	// Get
	got, err := GetLearning(db, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "WAL mode improves concurrent reads significantly" {
		t.Error("unexpected body")
	}

	// List all
	all, err := ListLearnings(db, p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 learnings, got %d", len(all))
	}

	// List by category
	arch, err := ListLearnings(db, p.ID, "architecture")
	if err != nil {
		t.Fatal(err)
	}
	if len(arch) != 1 {
		t.Errorf("expected 1 architecture learning, got %d", len(arch))
	}

	// Search
	results, err := SearchLearnings(db, "WAL")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}

	// Search no match
	none, err := SearchLearnings(db, "nonexistent123")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 results, got %d", len(none))
	}
}
