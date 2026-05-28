package db

import (
	"database/sql"
	"sort"
)

// Dependency-graph layering + critical path, computed server-side. The UI keeps
// only view-specific layout (within-layer ordering, coordinate math, rendering).

type GraphNode struct {
	ID       string `json:"id"`
	Layer    int    `json:"layer"`
	Critical bool   `json:"critical"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Graph struct {
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	MaxLayer int         `json:"max_layer"`
	HasCycle bool        `json:"has_cycle"`
}

// ComputeGraph builds the dependency DAG for a project: longest-path layer per
// connected task and the critical-path set. Cycles are detected and their
// back-edges are ignored for layering (deterministically, by task order) and
// reported via HasCycle — rather than the old client code silently pinning a
// mid-cycle node to layer 0.
func ComputeGraph(conn *sql.DB, projectID string, includeDone bool) (*Graph, error) {
	tasks, err := ListTasks(conn, ListTaskOpts{ProjectID: projectID})
	if err != nil {
		return nil, err
	}

	included := map[string]bool{}
	seqByID := map[string]int{}
	for _, t := range tasks {
		seqByID[t.ID] = t.Seq
		if includeDone || t.Status != "done" {
			included[t.ID] = true
		}
	}

	rawEdges, err := projectBlockEdges(conn, projectID)
	if err != nil {
		return nil, err
	}

	// Keep only edges between two included tasks, de-duplicated.
	seen := map[string]bool{}
	var edges []GraphEdge
	preds := map[string][]string{}
	connected := map[string]bool{}
	for _, e := range rawEdges {
		if !included[e.From] || !included[e.To] {
			continue
		}
		k := e.From + "\x00" + e.To
		if seen[k] {
			continue
		}
		seen[k] = true
		edges = append(edges, e)
		preds[e.To] = append(preds[e.To], e.From)
		connected[e.From] = true
		connected[e.To] = true
	}

	// Stable node order so cycle-breaking and tie-breaks are deterministic.
	nodeIDs := make([]string, 0, len(connected))
	for id := range connected {
		nodeIDs = append(nodeIDs, id)
	}
	lessBySeq := func(a, b string) bool {
		if seqByID[a] != seqByID[b] {
			return seqByID[a] < seqByID[b]
		}
		return a < b
	}
	sort.Slice(nodeIDs, func(i, j int) bool { return lessBySeq(nodeIDs[i], nodeIDs[j]) })
	for id := range preds {
		ps := preds[id]
		sort.Slice(ps, func(i, j int) bool { return lessBySeq(ps[i], ps[j]) })
	}

	// Longest-path layering via DFS. state: 0=unvisited, 1=on-stack, 2=done.
	layer := map[string]int{}
	state := map[string]int{}
	hasCycle := false
	var visit func(id string) int
	visit = func(id string) int {
		if state[id] == 2 {
			return layer[id]
		}
		state[id] = 1
		best := 0
		for _, p := range preds[id] {
			if state[p] == 1 {
				hasCycle = true // back-edge: ignore it for layering
				continue
			}
			if l := visit(p) + 1; l > best {
				best = l
			}
		}
		state[id] = 2
		layer[id] = best
		return best
	}
	for _, id := range nodeIDs {
		visit(id)
	}

	maxLayer := 0
	for _, l := range layer {
		if l > maxLayer {
			maxLayer = l
		}
	}

	critical := criticalPath(nodeIDs, preds, layer)

	nodes := make([]GraphNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nodes = append(nodes, GraphNode{ID: id, Layer: layer[id], Critical: critical[id]})
	}
	return &Graph{Nodes: nodes, Edges: edges, MaxLayer: maxLayer, HasCycle: hasCycle}, nil
}

// criticalPath walks back from the deepest node, always taking the
// strictly-lower-layer predecessor with the highest layer (the longest chain).
func criticalPath(nodeIDs []string, preds map[string][]string, layer map[string]int) map[string]bool {
	crit := map[string]bool{}
	end := ""
	deepest := -1
	for _, id := range nodeIDs { // nodeIDs is already in stable order
		if layer[id] > deepest {
			deepest = layer[id]
			end = id
		}
	}
	if end == "" {
		return crit
	}
	crit[end] = true
	cur := end
	for {
		best := ""
		bestLayer := -1
		for _, p := range preds[cur] {
			if crit[p] || layer[p] >= layer[cur] {
				continue // guard against cycles / non-decreasing steps
			}
			if layer[p] > bestLayer {
				bestLayer = layer[p]
				best = p
			}
		}
		if best == "" {
			break
		}
		crit[best] = true
		cur = best
	}
	return crit
}

func projectBlockEdges(conn *sql.DB, projectID string) ([]GraphEdge, error) {
	rows, err := conn.Query(`
		SELECT d.from_task_id, d.to_task_id
		FROM dependencies d
		WHERE d.dep_type = 'blocks'
		  AND (d.from_task_id IN (SELECT id FROM tasks WHERE project_id = ?)
		    OR d.to_task_id   IN (SELECT id FROM tasks WHERE project_id = ?))`,
		projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GraphEdge
	for rows.Next() {
		var e GraphEdge
		if err := rows.Scan(&e.From, &e.To); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
