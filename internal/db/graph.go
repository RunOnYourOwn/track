package db

import (
	"database/sql"
	"sort"
)

// Combined relationship graph, computed server-side. Nodes are laid out by their
// hierarchy depth (epic→feature→task→subtask); edges are tagged "contains"
// (parent→child) or "blocks" (dependency). The critical path is the longest
// chain through the blocks edges. The UI keeps only view-specific layout
// (within-layer ordering, coordinate math, rendering).

type GraphNode struct {
	ID       string `json:"id"`
	Layer    int    `json:"layer"` // hierarchy depth: epics at 0, their features at 1, ...
	Critical bool   `json:"critical"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // "contains" (parent→child) | "blocks" (dependency)
}

type Graph struct {
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	MaxLayer int         `json:"max_layer"`
	HasCycle bool        `json:"has_cycle"` // a dependency (blocks) cycle was detected
}

// ComputeGraph returns every task (honoring includeDone) as a node positioned by
// its hierarchy depth, with parent→child "contains" edges and "blocks"
// dependency edges. The critical path is the longest blocks chain; blocks cycles
// are detected (their back-edges ignored) and reported via HasCycle.
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
	// Parent links among included tasks (a parent filtered out by includeDone
	// makes its child a root in the graph).
	parentOf := map[string]string{}
	for _, t := range tasks {
		if included[t.ID] && t.ParentID != nil && included[*t.ParentID] {
			parentOf[t.ID] = *t.ParentID
		}
	}

	lessBySeq := func(a, b string) bool {
		if seqByID[a] != seqByID[b] {
			return seqByID[a] < seqByID[b]
		}
		return a < b
	}

	ids := make([]string, 0, len(included))
	for id := range included {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return lessBySeq(ids[i], ids[j]) })

	// Hierarchy depth. The parent chain is acyclic (parent_id is cycle-guarded on
	// write), so plain memoized recursion is safe.
	depth := map[string]int{}
	var hdepth func(id string) int
	hdepth = func(id string) int {
		if d, ok := depth[id]; ok {
			return d
		}
		p, ok := parentOf[id]
		if !ok {
			depth[id] = 0
			return 0
		}
		d := hdepth(p) + 1
		depth[id] = d
		return d
	}

	var edges []GraphEdge
	for _, id := range ids {
		if p, ok := parentOf[id]; ok {
			edges = append(edges, GraphEdge{From: p, To: id, Kind: "contains"})
		}
	}

	// Blocks dependency edges among included tasks, de-duplicated.
	rawEdges, err := projectBlockEdges(conn, projectID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	blocksPreds := map[string][]string{}
	blockNodes := map[string]bool{}
	for _, e := range rawEdges {
		if !included[e.From] || !included[e.To] {
			continue
		}
		k := e.From + "\x00" + e.To
		if seen[k] {
			continue
		}
		seen[k] = true
		edges = append(edges, GraphEdge{From: e.From, To: e.To, Kind: "blocks"})
		blocksPreds[e.To] = append(blocksPreds[e.To], e.From)
		blockNodes[e.From] = true
		blockNodes[e.To] = true
	}
	for id := range blocksPreds {
		ps := blocksPreds[id]
		sort.Slice(ps, func(i, j int) bool { return lessBySeq(ps[i], ps[j]) })
	}

	// Critical path = longest chain through the blocks edges. Computed over a
	// separate longest-path depth (NOT the hierarchy depth used for layout).
	blockIDs := make([]string, 0, len(blockNodes))
	for id := range blockNodes {
		blockIDs = append(blockIDs, id)
	}
	sort.Slice(blockIDs, func(i, j int) bool { return lessBySeq(blockIDs[i], blockIDs[j]) })

	bdepth := map[string]int{}
	state := map[string]int{}
	hasCycle := false
	var visit func(id string) int
	visit = func(id string) int {
		if state[id] == 2 {
			return bdepth[id]
		}
		state[id] = 1
		best := 0
		for _, p := range blocksPreds[id] {
			if state[p] == 1 {
				hasCycle = true // back-edge: ignore it
				continue
			}
			if l := visit(p) + 1; l > best {
				best = l
			}
		}
		state[id] = 2
		bdepth[id] = best
		return best
	}
	for _, id := range blockIDs {
		visit(id)
	}
	critical := criticalPath(blockIDs, blocksPreds, bdepth)

	maxLayer := 0
	nodes := make([]GraphNode, 0, len(ids))
	for _, id := range ids {
		d := hdepth(id)
		if d > maxLayer {
			maxLayer = d
		}
		nodes = append(nodes, GraphNode{ID: id, Layer: d, Critical: critical[id]})
	}

	return &Graph{Nodes: nodes, Edges: edges, MaxLayer: maxLayer, HasCycle: hasCycle}, nil
}

// criticalPath walks back from the deepest node, always taking the
// strictly-lower-depth predecessor with the highest depth (the longest chain).
func criticalPath(nodeIDs []string, preds map[string][]string, depthByID map[string]int) map[string]bool {
	crit := map[string]bool{}
	end := ""
	deepest := -1
	for _, id := range nodeIDs { // nodeIDs is already in stable order
		if depthByID[id] > deepest {
			deepest = depthByID[id]
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
		bestDepth := -1
		for _, p := range preds[cur] {
			if crit[p] || depthByID[p] >= depthByID[cur] {
				continue // guard against cycles / non-decreasing steps
			}
			if depthByID[p] > bestDepth {
				bestDepth = depthByID[p]
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
