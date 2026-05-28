package db

import "testing"

func TestComputeGraph(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "GR")

	mk := func(title string) string {
		tk, err := CreateTask(d, CreateTaskOpts{ProjectID: pid, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return tk.ID
	}
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	a, b, c, e := mk("A"), mk("B"), mk("C"), mk("D")
	// A blocks B, B blocks C, A blocks D  (from blocks to → from is predecessor)
	must(CreateDependency(d, a, b, "blocks", ""))
	must(CreateDependency(d, b, c, "blocks", ""))
	must(CreateDependency(d, a, e, "blocks", ""))

	g, err := ComputeGraph(d, pid, false)
	if err != nil {
		t.Fatal(err)
	}
	if g.HasCycle {
		t.Fatal("did not expect a cycle")
	}
	lay := map[string]int{}
	crit := map[string]bool{}
	for _, n := range g.Nodes {
		lay[n.ID] = n.Layer
		crit[n.ID] = n.Critical
	}
	if lay[a] != 0 || lay[b] != 1 || lay[e] != 1 || lay[c] != 2 {
		t.Fatalf("layers wrong: A=%d B=%d D=%d C=%d", lay[a], lay[b], lay[e], lay[c])
	}
	if g.MaxLayer != 2 {
		t.Fatalf("max layer: got %d want 2", g.MaxLayer)
	}
	// Longest chain is A→B→C; D is a side branch and not critical.
	if !crit[a] || !crit[b] || !crit[c] || crit[e] {
		t.Fatalf("critical path wrong: A=%v B=%v C=%v D=%v", crit[a], crit[b], crit[c], crit[e])
	}
	if len(g.Edges) != 3 {
		t.Fatalf("edges: got %d want 3", len(g.Edges))
	}

	// A 2-cycle must be detected and must not hang the layering.
	x, y := mk("X"), mk("Y")
	must(CreateDependency(d, x, y, "blocks", ""))
	must(CreateDependency(d, y, x, "blocks", ""))
	g2, err := ComputeGraph(d, pid, false)
	if err != nil {
		t.Fatal(err)
	}
	if !g2.HasCycle {
		t.Fatal("expected the X↔Y cycle to be detected")
	}
}
