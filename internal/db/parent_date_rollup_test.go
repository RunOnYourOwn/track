package db

import (
	"testing"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

// A parent_id cycle (only reachable via raw SQL — SetParentID guards against it)
// must not stack-overflow the date rollup.
func TestRollupParentDatesCycleSafe(t *testing.T) {
	bID, aID := "B", "A"
	tasks := []models.Task{
		{ID: "A", ParentID: &bID, CreatedAt: time.Now()},
		{ID: "B", ParentID: &aID, CreatedAt: time.Now()},
	}
	done := make(chan struct{})
	go func() { rollupParentDates(tasks); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("rollupParentDates did not terminate on a parent_id cycle")
	}
}

// CompleteTask with explicit hours must not overwrite already-logged time — the
// completion hours are added so the running total (and estimate accuracy) survive.
func TestCompleteTaskSumsLoggedHours(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "CHT")
	tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := LogTime(d, tk.ID, "", 3.0, "session"); err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, tk.ID, 2.0, "done"); err != nil {
		t.Fatal(err)
	}
	got, err := GetTask(d, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActualHours != 5.0 {
		t.Fatalf("actual_hours: got %v want 5 (3 logged + 2 completion)", got.ActualHours)
	}
}

// With no prior logged time, an explicit completion-hours value is set directly.
func TestCompleteTaskSetsHoursWhenNoLogs(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "CHN")
	tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, tk.ID, 4.0, ""); err != nil {
		t.Fatal(err)
	}
	got, err := GetTask(d, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActualHours != 4.0 {
		t.Fatalf("actual_hours: got %v want 4", got.ActualHours)
	}
}

// Epic/feature dates are derived from descendants on a full-project ListTasks:
// start = earliest descendant start, due = latest descendant due.
func TestRollupParentDates(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "ROL")

	mk := func(title, typ, parent, start, due string) string {
		tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: title, Type: typ, ParentID: parent, StartDate: start, DueDate: due})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return tk.ID
	}
	epic := mk("Epic", "epic", "", "", "")
	feat := mk("Feature", "feature", epic, "", "")
	mk("T1", "task", feat, "2026-03-01", "2026-03-10")
	mk("T2", "task", feat, "2026-02-15", "2026-03-20")

	tasks, err := ListTasks(d, ListTaskOpts{ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Taskish{}
	for _, tk := range tasks {
		byID[tk.ID] = Taskish{tk.StartDate, tk.DueDate}
	}
	for _, id := range []string{epic, feat} {
		got := byID[id]
		if got.start == nil || *got.start != "2026-02-15" {
			t.Fatalf("%s start: got %v, want 2026-02-15", id, deref(got.start))
		}
		if got.due == nil || *got.due != "2026-03-20" {
			t.Fatalf("%s due: got %v, want 2026-03-20", id, deref(got.due))
		}
	}
}

// A parent's date is settable while childless (e.g. ADO import) but rejected
// once it has descendants, since the value is then derived.
func TestParentDateWriteGuard(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "PDG")
	epic, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "E", Type: "epic"})
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskField(d, epic.ID, "due_date", "2026-07-01"); err != nil {
		t.Fatalf("childless epic due_date should be allowed: %v", err)
	}
	if _, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t", Type: "task", ParentID: epic.ID}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskField(d, epic.ID, "due_date", "2026-08-01"); err == nil {
		t.Fatal("epic with children should reject a manual due_date")
	}
}

type Taskish struct{ start, due *string }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
