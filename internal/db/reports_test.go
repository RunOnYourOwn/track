package db

import "testing"

func TestComputeVelocity(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "VEL")

	// Velocity accuracy is agent-axis: agent estimate (estimate_agent_minutes) vs
	// actual_hours. Use agent-minutes that map to clean hours (120m=2h, 240m=4h).
	mk := func(title string, agentMin int) string {
		tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: title, Type: "task", EstimateAgentMinutes: agentMin})
		if err != nil {
			t.Fatal(err)
		}
		return tk.ID
	}
	a, b := mk("A", 120), mk("B", 240)
	if err := CompleteTask(d, a, 3, ""); err != nil { // agent-est 2h / act 3h
		t.Fatal(err)
	}
	if err := CompleteTask(d, b, 4, ""); err != nil { // agent-est 4h / act 4h
		t.Fatal(err)
	}

	weeks, err := ComputeVelocity(d, pid, 4)
	if err != nil {
		t.Fatal(err)
	}
	// Both completed "now" → one ISO-week bucket.
	if len(weeks) != 1 {
		t.Fatalf("expected 1 week bucket, got %d", len(weeks))
	}
	w := weeks[0]
	if w.Done != 2 {
		t.Fatalf("done: got %d want 2", w.Done)
	}
	if w.EstAgentHours != 6 {
		t.Fatalf("agent-est hours: got %v want 6", w.EstAgentHours)
	}
	if w.ActHours != 7 {
		t.Fatalf("act hours: got %v want 7", w.ActHours)
	}
	// min(6,7)/max(6,7)*100 ≈ 85.7
	if w.Accuracy < 85 || w.Accuracy > 86 {
		t.Fatalf("accuracy: got %v want ~85.7", w.Accuracy)
	}
}

func TestRecordSnapshot(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "SNP")
	proj, err := GetProjectByPrefix(d, "SNP")
	if err != nil {
		t.Fatal(err)
	}

	mk := func(title string) string {
		tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: title, Type: "task"})
		if err != nil {
			t.Fatal(err)
		}
		return tk.ID
	}
	a := mk("A")
	mk("B")
	if err := CompleteTask(d, a, 0, ""); err != nil {
		t.Fatal(err)
	}

	snap, err := RecordSnapshot(d, proj)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Total != 2 || snap.Done != 1 || snap.Todo != 1 {
		t.Fatalf("snapshot counts wrong: total=%d done=%d todo=%d (want 2/1/1)", snap.Total, snap.Done, snap.Todo)
	}
	// It must be persisted, not just computed.
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM snapshots WHERE project_id = ?`, pid).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("snapshot not persisted: row count %d want 1", count)
	}
}
