package db

import (
	"math"
	"testing"
)

// ComputeInsights ports the former client-side analytics: throughput, the
// active-vs-lead cycle-time selection, estimation accuracy, the status
// distribution, and the WIP snapshot.
func TestComputeInsights(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "INS") // default WIP limit 3

	// t1: estimate 2h, actual 2h, done → 100% accuracy
	t1, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t1", EstimateHours: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := LogTime(d, t1.ID, "", 2.0, ""); err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, t1.ID, 0, ""); err != nil {
		t.Fatal(err)
	}

	// t2: estimate 4h, actual 1h, done → 25% accuracy
	t2, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t2", EstimateHours: 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := LogTime(d, t2.ID, "", 1.0, ""); err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, t2.ID, 0, ""); err != nil {
		t.Fatal(err)
	}

	// t3: in progress (counts toward WIP + distribution, not throughput-done)
	t3, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := MoveTask(d, t3.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}

	res, err := ComputeInsights(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	var pi *ProjectInsights
	for i := range res {
		if res[i].Prefix == "INS" {
			pi = &res[i]
		}
	}
	if pi == nil {
		t.Fatal("INS project not in insights result")
	}

	if pi.Throughput.Done != 2 || pi.Throughput.Total != 3 {
		t.Fatalf("throughput: got %+v, want done=2 total=3", pi.Throughput)
	}
	// Both completed tasks have logged active hours → "active" source.
	if pi.CycleTime.Source != "active" || pi.CycleTime.Count != 2 {
		t.Fatalf("cycle time: got %+v, want source=active count=2", pi.CycleTime)
	}
	if math.Abs(pi.CycleTime.AvgHours-1.5) > 1e-9 { // (2 + 1) / 2
		t.Fatalf("cycle avg: got %v, want 1.5", pi.CycleTime.AvgHours)
	}
	if pi.Accuracy.Count != 2 || math.Abs(pi.Accuracy.AvgPct-62.5) > 1e-9 { // (100 + 25) / 2
		t.Fatalf("accuracy: got %+v, want count=2 avg=62.5", pi.Accuracy)
	}
	if pi.Distribution.Done != 2 || pi.Distribution.InProgress != 1 || pi.Distribution.Todo != 0 {
		t.Fatalf("distribution: got %+v", pi.Distribution)
	}
	if pi.WIP.InProgress != 1 || pi.WIP.Limit != 3 {
		t.Fatalf("wip: got %+v, want in_progress=1 limit=3", pi.WIP)
	}
}
