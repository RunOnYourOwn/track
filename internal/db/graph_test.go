package db

import "testing"

func TestComputeGraph(t *testing.T) {
	d := OpenTestDB(t)
	pid := mkTestProject(t, d, "GR")

	mk := func(title, typ, parent string) string {
		opts := CreateTaskOpts{ProjectID: pid, Title: title, Type: typ}
		if parent != "" {
			opts.ParentID = parent
		}
		tk, err := CreateTask(d, opts)
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

	// Hierarchy: Epic > Feature > {A, B, C}
	epic := mk("Epic", "epic", "")
	feat := mk("Feature", "feature", epic)
	a := mk("A", "task", feat)
	b := mk("B", "task", feat)
	c := mk("C", "task", feat)
	// Dependencies (from blocks to): A → B → C
	must(CreateDependency(d, a, b, "blocks", ""))
	must(CreateDependency(d, b, c, "blocks", ""))

	g, err := ComputeGraph(d, pid, false)
	if err != nil {
		t.Fatal(err)
	}
	lay := map[string]int{}
	crit := map[string]bool{}
	for _, n := range g.Nodes {
		lay[n.ID] = n.Layer
		crit[n.ID] = n.Critical
	}

	// Layer = longest path over contains AND blocks edges. Epic→feat→A by
	// hierarchy (0,1,2); B and C are pushed right of their blockers (A blocks B,
	// B blocks C) rather than sharing A's column, so the chain flows left→right.
	if lay[epic] != 0 || lay[feat] != 1 || lay[a] != 2 || lay[b] != 3 || lay[c] != 4 {
		t.Fatalf("combined layer wrong: epic=%d feat=%d a=%d b=%d c=%d (want 0,1,2,3,4)", lay[epic], lay[feat], lay[a], lay[b], lay[c])
	}
	if g.MaxLayer != 4 {
		t.Fatalf("max layer: got %d want 4", g.MaxLayer)
	}

	// Edges: 4 contains (epic→feat, feat→a, feat→b, feat→c) + 2 blocks (a→b, b→c).
	contains, blocks := 0, 0
	for _, e := range g.Edges {
		switch e.Kind {
		case "contains":
			contains++
		case "blocks":
			blocks++
		default:
			t.Fatalf("unexpected edge kind %q", e.Kind)
		}
	}
	if contains != 4 || blocks != 2 {
		t.Fatalf("edges: got contains=%d blocks=%d want 4/2", contains, blocks)
	}

	// Critical path is the longest blocks chain A→B→C; hierarchy nodes aren't on it.
	if !crit[a] || !crit[b] || !crit[c] {
		t.Fatalf("blocks critical path wrong: a=%v b=%v c=%v", crit[a], crit[b], crit[c])
	}
	if crit[epic] || crit[feat] {
		t.Fatalf("hierarchy nodes should not be on the blocks critical path")
	}
	if g.HasCycle {
		t.Fatal("did not expect a cycle")
	}

	// A blocks cycle must be detected without hanging.
	x := mk("X", "task", feat)
	y := mk("Y", "task", feat)
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
