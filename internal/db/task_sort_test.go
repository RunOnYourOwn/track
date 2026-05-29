package db

import "testing"

// The project's task_sort mode drives ListTasks ordering server-side. Each mode
// has a distinct ORDER BY (taskOrderBy); this pins the observable order.
func TestTaskSortModes(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "SORT")

	mk := func(title, priority, due string) *sortRef {
		tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: title, Priority: priority, DueDate: due})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return &sortRef{id: tk.ID, seq: tk.Seq}
	}
	// Creation order is a(seq1), b(seq2), c(seq3).
	a := mk("A", "low", "2026-01-10")
	b := mk("B", "urgent", "2026-12-01")
	c := mk("C", "medium", "")

	order := func() []int {
		tasks, err := ListTasks(d, ListTaskOpts{ProjectID: pid})
		if err != nil {
			t.Fatal(err)
		}
		seqs := make([]int, len(tasks))
		for i, tk := range tasks {
			seqs[i] = tk.Seq
		}
		return seqs
	}
	setSort := func(mode string) {
		if err := UpdateProjectField(d, pid, "task_sort", mode); err != nil {
			t.Fatalf("set sort %s: %v", mode, err)
		}
	}
	want := func(got []int, exp ...int) {
		t.Helper()
		if len(got) != len(exp) {
			t.Fatalf("len: got %v want %v", got, exp)
		}
		for i := range exp {
			if got[i] != exp[i] {
				t.Fatalf("order: got %v want %v", got, exp)
			}
		}
	}

	// Default (no task_sort set yet) = priority: urgent(b), medium(c), low(a).
	want(order(), b.seq, c.seq, a.seq)

	setSort("priority")
	want(order(), b.seq, c.seq, a.seq)

	setSort("created") // by seq
	want(order(), a.seq, b.seq, c.seq)

	setSort("due") // soonest first, no-due last: a, b, c
	want(order(), a.seq, b.seq, c.seq)

	// manual: explicit sort_order asc, then priority. Set c<a<b.
	for id, so := range map[string]string{c.id: "1", a.id: "2", b.id: "3"} {
		if err := UpdateTaskField(d, id, "sort_order", so); err != nil {
			t.Fatalf("set sort_order: %v", err)
		}
	}
	setSort("manual")
	want(order(), c.seq, a.seq, b.seq)
}

type sortRef struct {
	id  string
	seq int
}
