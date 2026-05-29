package db

import (
	"database/sql"
	"strings"
	"sync"
	"testing"
)

func mkTestProject(t *testing.T, d *sql.DB, prefix string) string {
	t.Helper()
	p, err := CreateProject(d, prefix, prefix+" project", "", "", "", "", 3)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

// H4: a duplicate (project_id, seq) must be rejected by the unique index.
func TestCreateTaskSeqUniqueIndex(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "SEQ")

	first, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 {
		t.Fatalf("want seq 1, got %d", first.Seq)
	}

	_, err = d.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, created_at, updated_at)
		VALUES (?, ?, 1, 'dup', 'todo', 'medium', 'task', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, NewID(), pid)
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("expected UNIQUE violation on duplicate seq, got %v", err)
	}
}

// H4: sequential creates allocate distinct, increasing seqs.
func TestCreateTaskSeqSequential(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "SEQ2")
	for want := 1; want <= 3; want++ {
		tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t"})
		if err != nil {
			t.Fatal(err)
		}
		if tk.Seq != want {
			t.Fatalf("want seq %d, got %d", want, tk.Seq)
		}
	}
}

// H4: concurrent creates never produce duplicate seqs (the retry loop recovers
// from UNIQUE conflicts; transient SQLITE_BUSY is retried at the test level).
func TestCreateTaskConcurrentNoDuplicateSeq(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "RACE")

	const n = 8
	var wg sync.WaitGroup
	seqs := make([]int, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for attempt := 0; attempt < 200; attempt++ {
				tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "c"})
				if err == nil {
					seqs[i] = tk.Seq
					return
				}
				e := err.Error()
				if strings.Contains(e, "locked") || strings.Contains(strings.ToLower(e), "busy") {
					continue // transient without busy_timeout on the test handle
				}
				errs[i] = err
				return
			}
		}(i)
	}
	wg.Wait()

	seen := map[int]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("create %d: %v", i, errs[i])
		}
		if seqs[i] == 0 {
			t.Fatalf("create %d: no seq allocated", i)
		}
		if seen[seqs[i]] {
			t.Fatalf("duplicate seq %d allocated under concurrency", seqs[i])
		}
		seen[seqs[i]] = true
	}
}

// H1: create-path rejects invalid priority/type/source_type (the stored-XSS vector).
func TestCreateTaskRejectsInvalidFields(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "VAL")

	cases := []CreateTaskOpts{
		{ProjectID: pid, Title: "x", Priority: "bogus"},
		{ProjectID: pid, Title: "x", Type: "bogus"},
		{ProjectID: pid, Title: "x", SourceType: `<img src=x onerror=alert(1)>`},
	}
	for i, c := range cases {
		if _, err := CreateTask(d, c); err == nil {
			t.Fatalf("case %d: expected validation error, got nil", i)
		}
	}
	// "ado" must remain valid — ADO sync depends on it.
	if _, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "x", SourceType: "ado"}); err != nil {
		t.Fatalf("source_type 'ado' should be valid: %v", err)
	}
}

// XSS hardening: project prefixes are interpolated unescaped into display IDs
// and URLs, so the create path rejects unsafe prefixes and normalizes valid ones.
func TestCreateProjectValidatesPrefix(t *testing.T) {
	d := OpenTestDB(t)

	bad := []string{
		`<img src=x onerror=alert(1)>`,
		`A B`,         // space
		`A/B`,         // slash (breaks URL paths)
		`-LEADHYPHEN`, // must start alphanumeric
		"",
		"WAYTOOLONGPREFIXVALUE", // > 16 chars
	}
	for _, p := range bad {
		if _, err := CreateProject(d, p, "x", "", "", "", "", 3); err == nil {
			t.Fatalf("expected prefix %q to be rejected", p)
		}
	}

	// Valid prefixes are accepted and uppercased.
	p, err := CreateProject(d, "web-1", "Web", "", "", "", "", 3)
	if err != nil {
		t.Fatalf("valid prefix rejected: %v", err)
	}
	if p.Prefix != "WEB-1" {
		t.Fatalf("prefix not normalized to upper: got %q", p.Prefix)
	}
}

// completion_note is recorded on done + cancel; cancelled is a terminal status
// that doesn't block parent auto-close.
func TestCancelAndCompletionNote(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "CN")

	t1, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "ship it"})
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, t1.ID, 0, "shipped in v0.3"); err != nil {
		t.Fatal(err)
	}
	g1, _ := GetTask(d, t1.ID)
	if g1.Status != "done" || g1.CompletionNote == nil || *g1.CompletionNote != "shipped in v0.3" || g1.CompletedAt == nil {
		t.Fatalf("done+note wrong: status=%s note=%v completed=%v", g1.Status, g1.CompletionNote, g1.CompletedAt)
	}

	t2, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "drop it"})
	if err != nil {
		t.Fatal(err)
	}
	if err := CancelTask(d, t2.ID, "descoped for MVP"); err != nil {
		t.Fatal(err)
	}
	g2, _ := GetTask(d, t2.ID)
	if g2.Status != "cancelled" || g2.CompletionNote == nil || *g2.CompletionNote != "descoped for MVP" || g2.CompletedAt == nil {
		t.Fatalf("cancel+note wrong: status=%s note=%v completed=%v", g2.Status, g2.CompletionNote, g2.CompletedAt)
	}

	// A feature with one done + one cancelled child is fully closed → auto-closes.
	feat, _ := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "feature", Type: "feature"})
	c1, _ := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "c1", ParentID: feat.ID})
	c2, _ := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "c2", ParentID: feat.ID})
	if err := CompleteTask(d, c1.ID, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := CancelTask(d, c2.ID, "won't do"); err != nil {
		t.Fatal(err)
	}
	gf, _ := GetTask(d, feat.ID)
	if gf.Status != "done" {
		t.Fatalf("parent should auto-close when all children done/cancelled, got %q", gf.Status)
	}
}

// H5: deleting a non-empty project cascades instead of failing the FK check.
func TestDeleteProjectCascades(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "DEL")
	if _, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "child"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProject(d, pid); err != nil {
		t.Fatalf("DeleteProject with a task should succeed, got: %v", err)
	}
	var projects, tasks int
	d.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, pid).Scan(&projects)
	d.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id = ?`, pid).Scan(&tasks)
	if projects != 0 || tasks != 0 {
		t.Fatalf("cascade incomplete: projects=%d tasks=%d", projects, tasks)
	}
}

// M11: versioned migrations apply once, record version, and are idempotent.
func TestRunMigrationsAppliesOnceAndIsIdempotent(t *testing.T) {
	d := OpenTestDB(t)
	// Use a version above any real migration so this test exercises the runner
	// regardless of how many real migrations OpenTestDB has already applied.
	migs := []migration{
		{version: 9001, name: "add probe col", stmts: []string{`ALTER TABLE projects ADD COLUMN probe TEXT DEFAULT ''`}},
	}
	if err := runMigrations(d, migs); err != nil {
		t.Fatal(err)
	}
	var v int
	d.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&v)
	if v != 9001 {
		t.Fatalf("expected version 9001 recorded, got %d", v)
	}
	// Re-running must be a no-op (not re-apply the ALTER, which would error).
	if err := runMigrations(d, migs); err != nil {
		t.Fatalf("re-run should be a no-op, got: %v", err)
	}
}

// M11: a failing migration surfaces the error and is not recorded.
func TestRunMigrationsSurfacesErrors(t *testing.T) {
	d := OpenTestDB(t)
	migs := []migration{{version: 9002, name: "bad", stmts: []string{`THIS IS NOT SQL`}}}
	if err := runMigrations(d, migs); err == nil {
		t.Fatal("expected migration error to surface, got nil")
	}
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 9002`).Scan(&n)
	if n != 0 {
		t.Fatalf("failed migration must not be recorded, got %d rows", n)
	}
}

// M3/M4: reopening a done task clears completed_at and flags is_rework.
func TestReopenClearsCompletedAtAndFlagsRework(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "RW")
	tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if err := MoveTask(d, tk.ID, "done"); err != nil {
		t.Fatal(err)
	}
	doneTask, _ := GetTask(d, tk.ID)
	if doneTask.CompletedAt == nil {
		t.Fatal("completed_at should be set when done")
	}
	if doneTask.IsRework {
		t.Fatal("is_rework should be false before any reopen")
	}

	if err := MoveTask(d, tk.ID, "in_progress"); err != nil {
		t.Fatal(err)
	}
	reopened, _ := GetTask(d, tk.ID)
	if reopened.CompletedAt != nil {
		t.Fatalf("completed_at should be cleared on reopen, got %v", reopened.CompletedAt)
	}
	if !reopened.IsRework {
		t.Fatal("is_rework should be true after reopening a done task")
	}
}

// M4: flow efficiency is computable (and 0 with no completed work).
func TestComputeFlowEfficiencyEmpty(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "FE")
	if _, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "open"}); err != nil {
		t.Fatal(err)
	}
	eff, err := ComputeFlowEfficiency(d, pid)
	if err != nil {
		t.Fatal(err)
	}
	if eff != 0 {
		t.Fatalf("flow efficiency should be 0 with no completed tasks, got %v", eff)
	}
}

// H6: completing a task must not clobber manually logged hours.
func TestCompleteTaskPreservesLoggedHours(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "HRS")
	tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := LogTime(d, tk.ID, "", 2.0, "logged"); err != nil {
		t.Fatal(err)
	}
	if err := CompleteTask(d, tk.ID, 0, ""); err != nil {
		t.Fatal(err)
	}
	got, err := GetTask(d, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActualHours != 2.0 {
		t.Fatalf("logged hours clobbered on completion: want 2.0, got %v", got.ActualHours)
	}
}
