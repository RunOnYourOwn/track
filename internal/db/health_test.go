package db

import "testing"

// A project with one cleanly-completed, accurately-estimated task and nothing
// blocked/stale should hit all five health factors (100).
func TestComputeHealthAllFactors(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "HL") // default WIP limit 3
	proj, err := GetProjectByID(d, pid)
	if err != nil {
		t.Fatal(err)
	}

	// Accuracy is on the agent axis (estimate_agent_minutes vs actual_hours):
	// 120 agent-min = 2h, logged actual 2h → 100% accurate.
	tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "done", EstimateHours: 2, EstimateAgentMinutes: 120})
	if err != nil {
		t.Fatal(err)
	}
	if err := LogTime(d, tk.ID, "", 2.0, ""); err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, tk.ID, 0, ""); err != nil {
		t.Fatal(err)
	}

	tasks, err := ListTasks(d, ListTaskOpts{ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	score, f := ComputeHealth(proj, tasks, "HL")
	if score != 100 {
		t.Fatalf("score: got %d want 100 (factors %+v)", score, f)
	}
	if !f.BlockerFree || !f.WIPOk || !f.MakingProgress || !f.NoStale {
		t.Fatalf("expected all boolean factors true, got %+v", f)
	}
	if f.AccuracyPct != 100 {
		t.Fatalf("accuracy: got %v want 100", f.AccuracyPct)
	}
}
