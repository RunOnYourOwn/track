package db

import "testing"

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

type Taskish struct{ start, due *string }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
