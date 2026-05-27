package db

import (
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "SES", "Session Test", "build", "build", "", "{}", 3)

	// Start session
	s, err := StartSession(db, p.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "main" {
		t.Errorf("expected branch main, got %s", s.Branch)
	}
	if s.EndedAt != nil {
		t.Error("expected nil ended_at for open session")
	}

	// Get current session
	cur, err := GetCurrentSession(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur == nil || cur.ID != s.ID {
		t.Error("GetCurrentSession should return the open session")
	}

	// End session
	if err := EndSession(db, s.ID, "did some work"); err != nil {
		t.Fatal(err)
	}
	ended, _ := GetSession(db, s.ID)
	if ended.EndedAt == nil {
		t.Error("expected ended_at to be set")
	}
	if ended.Summary != "did some work" {
		t.Errorf("expected summary 'did some work', got %s", ended.Summary)
	}

	// No current session after end
	cur2, err := GetCurrentSession(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur2 != nil {
		t.Error("expected nil after ending session")
	}
}

func TestGetCurrentSessionEmpty(t *testing.T) {
	db := testDB(t)

	// No sessions at all — empty project
	s, err := GetCurrentSession(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Error("expected nil for no sessions")
	}
}

func TestListSessions(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "LST", "List Sessions", "build", "build", "", "{}", 3)

	s1, _ := StartSession(db, p.ID, "feat-1")
	EndSession(db, "not-real", "") // should be harmless
	// Insert s2 with a later timestamp to guarantee ordering
	EndSession(db, s1.ID, "done")
	_, _ = db.Exec(`UPDATE sessions SET started_at = '2026-01-01T00:00:00Z' WHERE id = ?`, s1.ID)
	s2, _ := StartSession(db, p.ID, "feat-2")

	sessions, err := ListSessions(db, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	// Most recent first
	if sessions[0].ID != s2.ID {
		t.Error("expected most recent session first")
	}

	// Limit
	limited, err := ListSessions(db, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Errorf("expected 1 session with limit=1, got %d", len(limited))
	}

	// Default limit (0 means 10)
	all, err := ListSessions(db, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 sessions with default limit, got %d", len(all))
	}
}

func TestLogTime(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "TIM", "Time Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Log time"})
	sess, _ := StartSession(db, p.ID, "main")

	if err := LogTime(db, task.ID, sess.ID, 1.5, "morning work"); err != nil {
		t.Fatal(err)
	}

	got, _ := GetTask(db, task.ID)
	if got.ActualHours != 1.5 {
		t.Errorf("expected 1.5 hours, got %f", got.ActualHours)
	}

	// Log more time
	if err := LogTime(db, task.ID, "", 0.5, "quick fix"); err != nil {
		t.Fatal(err)
	}
	got2, _ := GetTask(db, task.ID)
	if got2.ActualHours != 2.0 {
		t.Errorf("expected 2.0 hours, got %f", got2.ActualHours)
	}
}
