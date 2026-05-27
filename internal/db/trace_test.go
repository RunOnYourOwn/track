package db

import (
	"testing"
)

func TestRecordAndListCommits(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "TRC", "Trace Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Commit task"})

	// Record commits
	err := RecordCommit(db, task.ID, "abc1234", "track", "2026-05-27T10:00:00Z", "fix bug", []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	err = RecordCommit(db, task.ID, "def5678", "track", "2026-05-27T11:00:00Z", "add tests", []string{"main_test.go", "utils.go"})
	if err != nil {
		t.Fatal(err)
	}

	// List
	commits, err := ListCommitsForTask(db, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	// Most recent first
	if commits[0].CommitHash != "def5678" {
		t.Errorf("expected def5678 first, got %s", commits[0].CommitHash)
	}
	if commits[0].Message != "add tests" {
		t.Errorf("expected 'add tests', got %s", commits[0].Message)
	}

	// Replace (INSERT OR REPLACE)
	err = RecordCommit(db, task.ID, "abc1234", "track", "2026-05-27T10:00:00Z", "fix bug (amended)", []string{"main.go", "fix.go"})
	if err != nil {
		t.Fatal(err)
	}
	commits2, _ := ListCommitsForTask(db, task.ID)
	if len(commits2) != 2 {
		t.Errorf("expected 2 after replace, got %d", len(commits2))
	}

	// Empty task
	empty, err := ListCommitsForTask(db, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 for nonexistent task, got %d", len(empty))
	}
}

func TestRecordAndListDeploys(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "DPL", "Deploy Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Deploy task"})

	// Record deploy with defaults
	d, err := RecordDeploy(db, p.ID, "abc123", "v1.0.0", "", "", []string{task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d.Environment != "production" {
		t.Errorf("expected default production, got %s", d.Environment)
	}
	if d.TriggeredBy != "human" {
		t.Errorf("expected default human, got %s", d.TriggeredBy)
	}
	if d.Tag != "v1.0.0" {
		t.Errorf("expected tag v1.0.0, got %s", d.Tag)
	}

	// Record another
	d2, err := RecordDeploy(db, p.ID, "def456", "v1.1.0", "staging", "ci", []string{})
	if err != nil {
		t.Fatal(err)
	}
	if d2.Environment != "staging" {
		t.Errorf("expected staging, got %s", d2.Environment)
	}

	// Get deploy
	got, err := GetDeploy(db, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CommitHash != "abc123" {
		t.Errorf("expected abc123, got %s", got.CommitHash)
	}

	// List deploys
	deploys, err := ListDeploys(db, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deploys) != 2 {
		t.Fatalf("expected 2 deploys, got %d", len(deploys))
	}

	// List with limit
	limited, err := ListDeploys(db, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Errorf("expected 1 with limit, got %d", len(limited))
	}
}
