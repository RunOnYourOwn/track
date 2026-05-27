package db

import (
	"testing"
	"time"
)

func TestSessionStatsNoActivity(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)

	s, _ := StartSession(db, p.ID, "main")
	_ = EndSession(db, s.ID, "empty session")

	stats, err := GetSessionStats(db, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats == nil {
		t.Fatal("expected stats, got nil")
	}
	if stats.TotalHours != 0 {
		t.Errorf("expected 0 hours, got %f", stats.TotalHours)
	}
	if stats.TasksCompleted != 0 {
		t.Errorf("expected 0 completed, got %d", stats.TasksCompleted)
	}
	if stats.TasksTouched != 0 {
		t.Errorf("expected 0 touched, got %d", stats.TasksTouched)
	}
	if stats.CommitCount != 0 {
		t.Errorf("expected 0 commits, got %d", stats.CommitCount)
	}
	if stats.Tasks == nil {
		t.Error("expected non-nil Tasks slice")
	}
	if stats.Commits == nil {
		t.Error("expected non-nil Commits slice")
	}
}

func TestSessionStatsTimeLogged(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task A"})

	s1, _ := StartSession(db, p.ID, "main")
	s2, _ := StartSession(db, p.ID, "other")
	_ = EndSession(db, s2.ID, "")

	// Log time to session 1 (2 entries)
	_ = LogTime(db, task.ID, s1.ID, 1.5, "first")
	_ = LogTime(db, task.ID, s1.ID, 2.0, "second")

	// Log time to session 2 (should not appear in s1 stats)
	_ = LogTime(db, task.ID, s2.ID, 5.0, "other session")

	_ = EndSession(db, s1.ID, "")

	stats, err := GetSessionStats(db, s1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalHours != 3.5 {
		t.Errorf("expected 3.5 hours, got %f", stats.TotalHours)
	}
}

func TestSessionStatsTasksCompleted(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task1, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})
	task2, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 2"})

	// Session window: T-2h to T-30min
	now := time.Now().UTC()
	sessionStart := now.Add(-2 * time.Hour).Format(time.RFC3339)
	sessionEnd := now.Add(-30 * time.Minute).Format(time.RFC3339)

	// Insert session directly with controlled timestamps
	sid := NewID()
	db.Exec(`INSERT INTO sessions (id, project_id, branch, started_at, ended_at, summary) VALUES (?, ?, 'main', ?, ?, '')`,
		sid, p.ID, sessionStart, sessionEnd)

	// Task 1: completed WITHIN session window
	doneTime := now.Add(-1 * time.Hour).Format(time.RFC3339)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', ?)`,
		NewID(), task1.ID, doneTime)

	// Task 2: completed AFTER session window
	afterTime := now.Add(-10 * time.Minute).Format(time.RFC3339)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', ?)`,
		NewID(), task2.ID, afterTime)

	stats, err := GetSessionStats(db, sid)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TasksCompleted != 1 {
		t.Errorf("expected 1 completed, got %d", stats.TasksCompleted)
	}
}

func TestSessionStatsTasksTouched(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task1, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})
	task2, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 2"})
	task3, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 3"})

	now := time.Now().UTC()
	sessionStart := now.Add(-2 * time.Hour).Format(time.RFC3339)
	sessionEnd := now.Add(-30 * time.Minute).Format(time.RFC3339)

	sid := NewID()
	db.Exec(`INSERT INTO sessions (id, project_id, branch, started_at, ended_at, summary) VALUES (?, ?, 'main', ?, ?, '')`,
		sid, p.ID, sessionStart, sessionEnd)

	// Task 1: moved to in_progress within window
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'in_progress', ?)`,
		NewID(), task1.ID, now.Add(-90*time.Minute).Format(time.RFC3339))

	// Task 2: moved to in_progress then review within window
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'in_progress', ?)`,
		NewID(), task2.ID, now.Add(-100*time.Minute).Format(time.RFC3339))
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'review', ?)`,
		NewID(), task2.ID, now.Add(-80*time.Minute).Format(time.RFC3339))

	// Task 3: no changes during window (only before)
	_ = task3
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'in_progress', ?)`,
		NewID(), task3.ID, now.Add(-3*time.Hour).Format(time.RFC3339))

	stats, err := GetSessionStats(db, sid)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TasksTouched != 2 {
		t.Errorf("expected 2 touched, got %d", stats.TasksTouched)
	}
	if stats.TasksCompleted != 0 {
		t.Errorf("expected 0 completed, got %d", stats.TasksCompleted)
	}
}

func TestSessionStatsCommits(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})

	now := time.Now().UTC()
	sessionStart := now.Add(-2 * time.Hour).Format(time.RFC3339)
	sessionEnd := now.Add(-30 * time.Minute).Format(time.RFC3339)

	sid := NewID()
	db.Exec(`INSERT INTO sessions (id, project_id, branch, started_at, ended_at, summary) VALUES (?, ?, 'main', ?, ?, '')`,
		sid, p.ID, sessionStart, sessionEnd)

	// Two commits within window
	_ = RecordCommit(db, task.ID, "abc1234", "track",
		now.Add(-90*time.Minute).Format(time.RFC3339), "fix: stuff", []string{"a.go"})
	_ = RecordCommit(db, task.ID, "def5678", "track",
		now.Add(-60*time.Minute).Format(time.RFC3339), "feat: thing", []string{"b.go"})

	// One commit outside window
	_ = RecordCommit(db, task.ID, "ghi9012", "track",
		now.Add(-10*time.Minute).Format(time.RFC3339), "docs: update", []string{"c.go"})

	stats, err := GetSessionStats(db, sid)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CommitCount != 2 {
		t.Errorf("expected 2 commits, got %d", stats.CommitCount)
	}
	if len(stats.Commits) != 2 {
		t.Errorf("expected 2 commit objects, got %d", len(stats.Commits))
	}
}

func TestSessionStatsCycleTime(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})

	now := time.Now().UTC()
	sessionStart := now.Add(-2 * time.Hour).Format(time.RFC3339)
	sessionEnd := now.Add(-30 * time.Minute).Format(time.RFC3339)

	sid := NewID()
	db.Exec(`INSERT INTO sessions (id, project_id, branch, started_at, ended_at, summary) VALUES (?, ?, 'main', ?, ?, '')`,
		sid, p.ID, sessionStart, sessionEnd)

	// Task started in_progress 3 hours ago (before session)
	ipTime := now.Add(-3 * time.Hour)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'in_progress', ?)`,
		NewID(), task.ID, ipTime.Format(time.RFC3339))

	// Task completed 1 hour ago (within session)
	doneTime := now.Add(-1 * time.Hour)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', ?)`,
		NewID(), task.ID, doneTime.Format(time.RFC3339))

	stats, err := GetSessionStats(db, sid)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TasksCompleted != 1 {
		t.Fatalf("expected 1 completed, got %d", stats.TasksCompleted)
	}
	if len(stats.Tasks) == 0 {
		t.Fatal("expected at least one task activity")
	}

	// Cycle time should be ~2 hours (7200 seconds) — from in_progress to done
	var found bool
	for _, ta := range stats.Tasks {
		if ta.TaskID == task.ID && ta.CycleTimeSec != nil {
			found = true
			expected := int64(doneTime.Sub(ipTime).Seconds())
			diff := *ta.CycleTimeSec - expected
			if diff < -5 || diff > 5 {
				t.Errorf("expected cycle time ~%d, got %d", expected, *ta.CycleTimeSec)
			}
		}
	}
	if !found {
		t.Error("expected task with cycle time")
	}
}

func TestSessionStatsOngoingSession(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})

	// Start session but don't end it
	s, _ := StartSession(db, p.ID, "main")

	// Log time and make a commit after session start
	_ = LogTime(db, task.ID, s.ID, 1.0, "work")
	_ = RecordCommit(db, task.ID, "aaa1111", "track", Now(), "wip", []string{"x.go"})

	stats, err := GetSessionStats(db, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalHours != 1.0 {
		t.Errorf("expected 1.0 hours, got %f", stats.TotalHours)
	}
	if stats.CommitCount != 1 {
		t.Errorf("expected 1 commit, got %d", stats.CommitCount)
	}
}

func TestSessionStatsIsolatesProjects(t *testing.T) {
	db := testDB(t)
	pA, _ := CreateProject(db, "AAA", "Project A", "build", "build", "", "{}", 3)
	pB, _ := CreateProject(db, "BBB", "Project B", "build", "build", "", "{}", 3)

	taskA, _ := CreateTask(db, CreateTaskOpts{ProjectID: pA.ID, Title: "Task A"})

	now := time.Now().UTC()
	sessionStart := now.Add(-2 * time.Hour).Format(time.RFC3339)
	sessionEnd := now.Add(-30 * time.Minute).Format(time.RFC3339)

	// Session for project B
	sidB := NewID()
	db.Exec(`INSERT INTO sessions (id, project_id, branch, started_at, ended_at, summary) VALUES (?, ?, 'main', ?, ?, '')`,
		sidB, pB.ID, sessionStart, sessionEnd)

	// Activity only on project A's task (during B's session window)
	db.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'done', ?)`,
		NewID(), taskA.ID, now.Add(-1*time.Hour).Format(time.RFC3339))
	_ = RecordCommit(db, taskA.ID, "iso1234", "track",
		now.Add(-1*time.Hour).Format(time.RFC3339), "commit on A", []string{"a.go"})

	stats, err := GetSessionStats(db, sidB)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TasksCompleted != 0 {
		t.Errorf("expected 0 completed for project B, got %d", stats.TasksCompleted)
	}
	if stats.CommitCount != 0 {
		t.Errorf("expected 0 commits for project B, got %d", stats.CommitCount)
	}
}

func TestGetSessionStatsBatch(t *testing.T) {
	db := testDB(t)
	p, _ := CreateProject(db, "TST", "Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Task 1"})

	s1, _ := StartSession(db, p.ID, "main")
	_ = LogTime(db, task.ID, s1.ID, 2.0, "work")
	_ = EndSession(db, s1.ID, "done")

	s2, _ := StartSession(db, p.ID, "feat")
	_ = LogTime(db, task.ID, s2.ID, 1.5, "more work")
	_ = EndSession(db, s2.ID, "also done")

	s3, _ := StartSession(db, p.ID, "empty")
	_ = EndSession(db, s3.ID, "nothing")

	batch, err := GetSessionStatsBatch(db, []string{s1.ID, s2.ID, s3.ID, "nonexistent-id"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(batch))
	}
	if batch["nonexistent-id"].TotalHours != 0 {
		t.Errorf("nonexistent: expected 0 hours, got %f", batch["nonexistent-id"].TotalHours)
	}
	if batch[s1.ID].TotalHours != 2.0 {
		t.Errorf("s1 hours: expected 2.0, got %f", batch[s1.ID].TotalHours)
	}
	if batch[s2.ID].TotalHours != 1.5 {
		t.Errorf("s2 hours: expected 1.5, got %f", batch[s2.ID].TotalHours)
	}
	if batch[s3.ID].TotalHours != 0 {
		t.Errorf("s3 hours: expected 0, got %f", batch[s3.ID].TotalHours)
	}
}

func TestSessionStatsNotFound(t *testing.T) {
	db := testDB(t)

	stats, err := GetSessionStats(db, "nonexistent-id")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil stats, got %+v", stats)
	}
}
