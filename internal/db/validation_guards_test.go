package db

import (
	"strings"
	"testing"
)

func TestLogTimeRejectsNonPositiveHours(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "LOG")
	tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []float64{0, -1.5} {
		if err := LogTime(d, tk.ID, "", h, ""); err == nil {
			t.Fatalf("LogTime(%g) should error", h)
		}
	}
	if err := LogTime(d, tk.ID, "", 1.5, ""); err != nil {
		t.Fatalf("LogTime(1.5) should succeed, got %v", err)
	}
}

func TestUpdateSprintStatusValidates(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "SPR")
	sp, err := CreateSprint(d, CreateSprintOpts{ProjectID: pid, Name: "S1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateSprintStatus(d, sp.ID, "bogus"); err == nil {
		t.Fatal("invalid sprint status should error")
	}
	if err := UpdateSprintStatus(d, sp.ID, "active"); err != nil {
		t.Fatalf("valid status should succeed: %v", err)
	}
	if err := UpdateSprintStatus(d, "nope", "active"); err == nil {
		t.Fatal("unknown sprint id should error")
	}
}

func TestResolveDecisionGuardsAlreadyDecided(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "DEC")
	dec, err := CreateDecision(d, CreateDecisionOpts{ProjectID: pid, Title: "pick db"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ResolveDecision(d, dec.ID, "sqlite", "single user"); err != nil {
		t.Fatalf("first resolve should succeed: %v", err)
	}
	// Second resolve must be rejected, not silently overwrite the rationale.
	err = ResolveDecision(d, dec.ID, "postgres", "changed my mind")
	if err == nil || !strings.Contains(err.Error(), "already decided") {
		t.Fatalf("second resolve should report already-decided, got %v", err)
	}
	if err := ResolveDecision(d, "missing", "x", "y"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("resolve missing should report not-found, got %v", err)
	}
}
