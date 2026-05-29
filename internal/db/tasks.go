package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

type CreateTaskOpts struct {
	ProjectID            string
	Title                string
	Description          string
	Priority             string
	Type                 string
	EstimateSize         string
	EstimateHours        float64
	EstimateAgentMinutes int
	ParentID             string
	SourceType           string
	AgentContext         string
	Tags                 string
	StartDate            string
	DueDate              string
}

func CreateTask(d *sql.DB, opts CreateTaskOpts) (*models.Task, error) {
	priority := opts.Priority
	if priority == "" {
		priority = "medium"
	} else if !validPriorities[priority] {
		return nil, fmt.Errorf("invalid priority %q", priority)
	}
	taskType := opts.Type
	if taskType == "" {
		taskType = "task"
	} else if !validTypes[taskType] {
		return nil, fmt.Errorf("invalid type %q", taskType)
	}
	source := opts.SourceType
	if source == "" {
		source = "planned"
	} else if !validSourceTypes[source] {
		return nil, fmt.Errorf("invalid source_type %q", source)
	}
	agentCtx := opts.AgentContext
	if agentCtx == "" {
		agentCtx = "{}"
	}
	tags := opts.Tags
	if tags == "" {
		tags = "[]"
	}

	var parentID *string
	if opts.ParentID != "" {
		parentID = &opts.ParentID
	}
	var startDate *string
	if opts.StartDate != "" {
		startDate = &opts.StartDate
	}
	var dueDate *string
	if opts.DueDate != "" {
		dueDate = &opts.DueDate
	}

	// Allocate seq inside the transaction and retry on a UNIQUE(project_id, seq)
	// collision (possible when concurrent in-process callers or other processes
	// race the MAX(seq) read). The unique index is the authority; we just retry.
	var id string
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		id = NewID()
		now := Now()

		tx, err := d.Begin()
		if err != nil {
			return nil, err
		}

		var max sql.NullInt64
		if err := tx.QueryRow(`SELECT MAX(seq) FROM tasks WHERE project_id = ?`, opts.ProjectID).Scan(&max); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("next seq: %w", err)
		}
		seq := 1
		if max.Valid {
			seq = int(max.Int64) + 1
		}

		_, err = tx.Exec(`INSERT INTO tasks (id, project_id, seq, title, description, status, priority, type, estimate_size, estimate_hours, estimate_agent_minutes, parent_id, sort_order, source_type, agent_context, tags, start_date, due_date, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'todo', ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?)`,
			id, opts.ProjectID, seq, opts.Title, opts.Description, priority, taskType, opts.EstimateSize, opts.EstimateHours, opts.EstimateAgentMinutes, parentID, source, agentCtx, tags, startDate, dueDate, now, now)
		if err != nil {
			tx.Rollback()
			if isSeqConflict(err) {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("create task: %w", err)
		}

		if _, err := tx.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, 'todo', ?)`, NewID(), id, now); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("create status history: %w", err)
		}

		if err := tx.Commit(); err != nil {
			if isSeqConflict(err) {
				lastErr = err
				continue
			}
			return nil, err
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, fmt.Errorf("create task: seq allocation conflict: %w", lastErr)
	}

	if opts.ParentID != "" && source != "bug" && source != "debt" {
		if err := autoReopenParent(d, opts.ParentID); err != nil {
			return nil, err
		}
	}

	return GetTask(d, id)
}

// isSeqConflict reports whether err is a UNIQUE(project_id, seq) violation,
// which the create loop retries with a freshly recomputed seq. Match the seq
// column specifically — a different UNIQUE violation (e.g. a duplicate id) is a
// real error, not something to retry.
func isSeqConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "tasks.seq")
}

func autoReopenParent(db *sql.DB, parentID string) error {
	return autoReopenParentWalk(db, parentID, map[string]bool{})
}

func autoReopenParentWalk(db *sql.DB, parentID string, visited map[string]bool) error {
	if visited[parentID] {
		return nil
	}
	visited[parentID] = true

	var status string
	var grandparentID sql.NullString
	err := db.QueryRow(`SELECT status, parent_id FROM tasks WHERE id = ?`, parentID).Scan(&status, &grandparentID)
	if err == sql.ErrNoRows {
		return nil // parent gone — nothing to reopen
	}
	if err != nil {
		return err
	}
	if status != "done" {
		return nil
	}
	if err := MoveTask(db, parentID, "in_progress"); err != nil {
		return err
	}
	if grandparentID.Valid && grandparentID.String != "" {
		return autoReopenParentWalk(db, grandparentID.String, visited)
	}
	return nil
}

func GetTask(db *sql.DB, id string) (*models.Task, error) {
	row := db.QueryRow(taskSelect+` WHERE t.id = ?`, id)
	return scanTask(row)
}

func GetTaskByDisplayID(db *sql.DB, prefix string, seq int) (*models.Task, error) {
	row := db.QueryRow(taskSelect+` WHERE p.prefix = ? AND t.seq = ?`, strings.ToUpper(prefix), seq)
	return scanTask(row)
}

type ListTaskOpts struct {
	ProjectID string
	Status    []string
	Priority  []string
	Type      string
	ParentID  string
}

// priorityRank orders tasks urgent→high→medium→low; reused by several sort modes.
const priorityRank = "CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 END"

// taskOrderBy maps a project's task_sort mode to its SQL ORDER BY clause — the one
// server-side source of truth for task ordering (the UI just renders the result).
// An unknown mode falls back to the priority default.
func taskOrderBy(mode string) string {
	switch mode {
	case "manual":
		return "t.sort_order, " + priorityRank + ", t.seq"
	case "created":
		return "t.seq"
	case "due":
		return "CASE WHEN t.due_date IS NULL OR t.due_date = '' THEN 1 ELSE 0 END, t.due_date, " + priorityRank + ", t.seq"
	default: // "priority"
		return priorityRank + ", t.sort_order, t.seq"
	}
}

func ListTasks(db *sql.DB, opts ListTaskOpts) ([]models.Task, error) {
	query := taskSelect
	var args []any
	var conditions []string

	if opts.ProjectID != "" {
		conditions = append(conditions, "t.project_id = ?")
		args = append(args, opts.ProjectID)
	}
	if len(opts.Status) > 0 {
		placeholders := strings.Repeat("?,", len(opts.Status))
		placeholders = placeholders[:len(placeholders)-1]
		conditions = append(conditions, fmt.Sprintf("t.status IN (%s)", placeholders))
		for _, s := range opts.Status {
			args = append(args, s)
		}
	}
	if len(opts.Priority) > 0 {
		placeholders := strings.Repeat("?,", len(opts.Priority))
		placeholders = placeholders[:len(placeholders)-1]
		conditions = append(conditions, fmt.Sprintf("t.priority IN (%s)", placeholders))
		for _, p := range opts.Priority {
			args = append(args, p)
		}
	}
	if opts.Type != "" {
		conditions = append(conditions, "t.type = ?")
		args = append(args, opts.Type)
	}
	if opts.ParentID != "" {
		conditions = append(conditions, "t.parent_id = ?")
		args = append(args, opts.ParentID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	// Order by the project's configured sort mode. Only applies when scoped to a
	// single project (a mixed-project list has no single mode, so it falls back to
	// the priority default).
	sortMode := "priority"
	if opts.ProjectID != "" {
		_ = db.QueryRow(`SELECT task_sort FROM projects WHERE id = ?`, opts.ProjectID).Scan(&sortMode)
	}
	query += " ORDER BY " + taskOrderBy(sortMode)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Container (epic/feature) dates and estimates are derived from their
	// descendants, not set directly. Roll them up only for a full, unfiltered
	// project fetch — a filtered subset would miss descendants and compute wrong
	// spans/sums.
	if opts.ProjectID != "" && len(opts.Status) == 0 && len(opts.Priority) == 0 && opts.Type == "" && opts.ParentID == "" {
		rollupParentDerived(tasks)
	}
	return tasks, nil
}

// rollupParentDerived sets each container (epic/feature) task's descendant-derived
// fields, so a parent stays consistent without entering the same value in two
// places. Leaf tasks are left as-is. Derived fields:
//   - start_date = earliest descendant start (its start_date, else creation date)
//   - due_date   = latest descendant due_date (empty if none)
//   - estimate_hours / estimate_agent_minutes = sum over all descendants
//
// estimate_size (T-shirt) is NOT rolled up — it's categorical, not additive.
// Dates are YYYY-MM-DD strings, which sort lexicographically.
func rollupParentDerived(tasks []models.Task) {
	childIdx := map[string][]int{}
	for i := range tasks {
		if p := tasks[i].ParentID; p != nil && *p != "" {
			childIdx[*p] = append(childIdx[*p], i)
		}
	}
	type agg struct {
		start, due  string
		estHours    float64
		estAgentMin int
	}
	memo := map[string]agg{}
	visiting := map[string]bool{}
	var compute func(i int) agg
	compute = func(i int) agg {
		t := &tasks[i]
		if a, ok := memo[t.ID]; ok {
			return a
		}
		// memo is set after recursing into children, so guard against a parent_id
		// cycle (re-entering an in-progress node) to avoid unbounded recursion.
		if visiting[t.ID] {
			return agg{}
		}
		visiting[t.ID] = true
		kids := childIdx[t.ID]
		var a agg
		if len(kids) == 0 {
			if t.StartDate != nil && *t.StartDate != "" {
				a.start = *t.StartDate
			} else {
				a.start = t.CreatedAt.Format("2006-01-02")
			}
			if t.DueDate != nil {
				a.due = *t.DueDate
			}
			a.estHours = t.EstimateHours
			a.estAgentMin = t.EstimateAgentMinutes
		} else {
			for _, ci := range kids {
				cs := compute(ci)
				if cs.start != "" && (a.start == "" || cs.start < a.start) {
					a.start = cs.start
				}
				if cs.due != "" && (a.due == "" || cs.due > a.due) {
					a.due = cs.due
				}
				a.estHours += cs.estHours
				a.estAgentMin += cs.estAgentMin
			}
			if a.start != "" {
				s := a.start
				t.StartDate = &s
			}
			if a.due != "" {
				d := a.due
				t.DueDate = &d
			} else {
				t.DueDate = nil
			}
			t.EstimateHours = a.estHours
			t.EstimateAgentMinutes = a.estAgentMin
		}
		memo[t.ID] = a
		return a
	}
	for i := range tasks {
		compute(i)
	}
}

// OrderedStatuses is the canonical task-status vocabulary in display order — the
// single source of truth shipped to the web UI via GET /api/meta, so the UI no
// longer hard-codes its own copy. validStatuses (membership) derives from it.
var OrderedStatuses = []string{
	"todo", "in_progress", "blocked", "done", "cancelled",
	"waiting_review", "waiting_external", "waiting_dependency",
}

// OrderedPriorities is the canonical priority vocabulary, highest-urgency first;
// the slice index is the sort rank (mirrored by priorityRank in SQL).
var OrderedPriorities = []string{"urgent", "high", "medium", "low"}

var validStatuses = sliceToSet(OrderedStatuses)
var validPriorities = sliceToSet(OrderedPriorities)

func sliceToSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// terminalStatuses are the "closed" states — a task in one of these is neither
// open/active work nor counted as completed throughput (cancelled) vs completed (done).
func isTerminalStatus(s string) bool { return s == "done" || s == "cancelled" }

var validTypes = map[string]bool{
	"task": true, "feature": true, "epic": true, "bug": true, "debt": true,
}

var validSourceTypes = map[string]bool{
	"planned": true, "discovered": true, "stakeholder": true,
	"bug": true, "debt": true, "ado": true,
}

func MoveTask(db *sql.DB, id, status string) error {
	if !validStatuses[status] {
		return validationErrf("invalid status %q", status)
	}
	if err := moveTaskNoAutoClose(db, id, status); err != nil {
		return err
	}
	if isTerminalStatus(status) {
		if err := autoCloseParent(db, id); err != nil {
			return err
		}
	}
	return nil
}

func moveTaskNoAutoClose(d *sql.DB, id, status string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Capture the previous status to detect rework (done → not-done reopen).
	var prevStatus string
	if err := tx.QueryRow(`SELECT status FROM tasks WHERE id = ?`, id).Scan(&prevStatus); err != nil {
		return err
	}

	now := Now()
	if _, err := tx.Exec(`UPDATE task_status_history SET exited_at = ? WHERE task_id = ? AND exited_at IS NULL`, now, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO task_status_history (id, task_id, status, entered_at) VALUES (?, ?, ?, ?)`, NewID(), id, status, now); err != nil {
		return err
	}

	if isTerminalStatus(status) {
		// done OR cancelled records a close timestamp in completed_at.
		if _, err := tx.Exec(`UPDATE tasks SET status = ?, updated_at = ?, completed_at = ? WHERE id = ?`,
			status, now, now, id); err != nil {
			return err
		}
	} else {
		// Clear completed_at on any non-terminal status; mark rework when a
		// previously completed task is reopened (the only signal feeding rework_rate).
		reopened := prevStatus == "done"
		if _, err := tx.Exec(`UPDATE tasks SET status = ?, updated_at = ?, completed_at = NULL, is_rework = CASE WHEN ? THEN 1 ELSE is_rework END WHERE id = ?`,
			status, now, reopened, id); err != nil {
			return err
		}
	}

	if status == "done" {
		// Only derive actual_hours from the in-progress time when the user hasn't
		// logged time manually. LogTime accumulates real hours into actual_hours;
		// overwriting them here (or in CompleteTask) would silently discard them.
		var logged int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM time_entries WHERE task_id = ?`, id).Scan(&logged); err != nil {
			return err
		}
		if logged == 0 {
			hours, err := computeActiveHours(tx, id)
			if err != nil {
				return err
			}
			if hours > 0 {
				if _, err := tx.Exec(`UPDATE tasks SET actual_hours = ? WHERE id = ?`, hours, id); err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit()
}

func computeActiveHours(tx *sql.Tx, taskID string) (float64, error) {
	var totalSeconds sql.NullFloat64
	err := tx.QueryRow(`
		SELECT SUM(
			(julianday(exited_at) - julianday(entered_at)) * 86400
		)
		FROM task_status_history
		WHERE task_id = ? AND status = 'in_progress' AND exited_at IS NOT NULL
	`, taskID).Scan(&totalSeconds)
	if err != nil {
		return 0, err
	}
	if !totalSeconds.Valid || totalSeconds.Float64 <= 0 {
		return 0, nil
	}
	return totalSeconds.Float64 / 3600.0, nil
}

func autoCloseParent(db *sql.DB, childID string) error {
	return autoCloseParentWalk(db, childID, map[string]bool{})
}

func autoCloseParentWalk(db *sql.DB, childID string, visited map[string]bool) error {
	if visited[childID] {
		return nil
	}
	visited[childID] = true

	var parentID sql.NullString
	if err := db.QueryRow(`SELECT parent_id FROM tasks WHERE id = ?`, childID).Scan(&parentID); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if !parentID.Valid || parentID.String == "" {
		return nil
	}
	if visited[parentID.String] {
		return nil
	}

	var pending int
	// Surface a scan error rather than treating pending as 0 — otherwise a
	// transient DB error would auto-close a parent that still has open children.
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE parent_id = ? AND status NOT IN ('done','cancelled')`, parentID.String).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return nil
	}

	visited[parentID.String] = true
	if err := moveTaskNoAutoClose(db, parentID.String, "done"); err != nil {
		return err
	}
	return autoCloseParentWalk(db, parentID.String, visited)
}

func CompleteTask(db *sql.DB, id string, actualHours float64, note string) error {
	if err := MoveTask(db, id, "done"); err != nil {
		return err
	}
	if actualHours > 0 {
		// If the task already has manually logged time, record the completion hours
		// as another time entry (which accumulates into actual_hours) instead of
		// overwriting the running total — overwriting silently discarded logged time
		// and corrupted estimate-accuracy reporting.
		var logged int
		if err := db.QueryRow(`SELECT COUNT(*) FROM time_entries WHERE task_id = ?`, id).Scan(&logged); err != nil {
			return err
		}
		if logged > 0 {
			if err := LogTime(db, id, "", actualHours, note); err != nil {
				return err
			}
		} else {
			now := Now()
			if _, err := db.Exec(`UPDATE tasks SET actual_hours = ?, updated_at = ? WHERE id = ?`, actualHours, now, id); err != nil {
				return err
			}
		}
	}
	if note != "" {
		return UpdateTaskField(db, id, "completion_note", note)
	}
	return nil
}

// CancelTask marks a task terminally cancelled (not completed), recording an
// optional reason in completion_note. completed_at is set as the close timestamp.
func CancelTask(db *sql.DB, id, note string) error {
	if err := MoveTask(db, id, "cancelled"); err != nil {
		return err
	}
	if note != "" {
		return UpdateTaskField(db, id, "completion_note", note)
	}
	return nil
}

func SetParentID(d *sql.DB, id, parentID string) error {
	if parentID == "" {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := d.Exec(`UPDATE tasks SET parent_id = NULL, updated_at = ? WHERE id = ?`, now, id)
		return err
	}
	if parentID == id {
		return fmt.Errorf("task cannot be its own parent")
	}

	// Skip if already set to this parent
	var current sql.NullString
	if err := d.QueryRow(`SELECT parent_id FROM tasks WHERE id = ?`, id).Scan(&current); err == nil {
		if current.Valid && current.String == parentID {
			return nil
		}
	}

	// Walk up the ancestor chain from parentID to detect cycles
	cur := parentID
	for i := 0; i < 100; i++ {
		var ancestor sql.NullString
		err := d.QueryRow(`SELECT parent_id FROM tasks WHERE id = ?`, cur).Scan(&ancestor)
		if err != nil || !ancestor.Valid || ancestor.String == "" {
			break
		}
		if ancestor.String == id {
			return fmt.Errorf("circular parent_id: %s is an ancestor of %s", id, parentID)
		}
		cur = ancestor.String
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.Exec(`UPDATE tasks SET parent_id = ?, updated_at = ? WHERE id = ?`, parentID, now, id)
	return err
}

var allowedTaskFields = map[string]bool{
	"title": true, "description": true, "type": true, "priority": true,
	"agent_context": true, "due_date": true, "start_date": true, "sort_order": true,
	"estimate_size": true, "estimate_hours": true, "estimate_agent_minutes": true,
	"actual_hours": true, "tags": true, "completion_note": true,
}

func UpdateTaskField(d *sql.DB, id, field, value string) error {
	if !allowedTaskFields[field] {
		return fmt.Errorf("UpdateTaskField: disallowed field %q", field)
	}
	switch field {
	case "priority":
		if !validPriorities[value] {
			return fmt.Errorf("invalid priority %q", value)
		}
	case "type":
		if !validTypes[value] {
			return fmt.Errorf("invalid type %q", value)
		}
	case "start_date", "due_date", "estimate_hours", "estimate_agent_minutes":
		// Once a container has descendants these fields are derived (see
		// rollupParentDerived), so setting them directly is rejected. A childless
		// epic/feature can still hold a value (e.g. an ADO-imported placeholder).
		// estimate_size (T-shirt) is intentionally NOT here — it isn't rolled up.
		var typ string
		if err := d.QueryRow(`SELECT type FROM tasks WHERE id = ?`, id).Scan(&typ); err != nil {
			return err
		}
		if typ == "epic" || typ == "feature" {
			var hasChild bool
			_ = d.QueryRow(`SELECT EXISTS(SELECT 1 FROM tasks WHERE parent_id = ?)`, id).Scan(&hasChild)
			if hasChild {
				return validationErrf("%s is derived from descendant tasks for this %s; set it on the tasks instead", field, typ)
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	query := fmt.Sprintf(`UPDATE tasks SET %s = ?, updated_at = ? WHERE id = ?`, field)
	_, err := d.Exec(query, value, now, id)
	return err
}

func SyncAgentContext(d *sql.DB, id, context string) error {
	_, err := d.Exec(`UPDATE tasks SET agent_context = ? WHERE id = ?`, context, id)
	return err
}

// DeleteTask removes a task and detaches/cleans its references. Returns
// sql.ErrNoRows if no task with that id exists (so callers can map a 404). Every
// cleanup statement's error is surfaced — a failed cleanup rolls the whole
// deletion back rather than committing a partial, orphaning state.
func DeleteTask(d *sql.DB, id string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	cleanup := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM task_status_history WHERE task_id = ?`, []any{id}},
		{`DELETE FROM dependencies WHERE from_task_id = ? OR to_task_id = ?`, []any{id, id}},
		{`DELETE FROM time_entries WHERE task_id = ?`, []any{id}},
		{`DELETE FROM task_commits WHERE task_id = ?`, []any{id}},
		{`DELETE FROM sprint_tasks WHERE task_id = ?`, []any{id}},
		// cross_project_deps has non-nullable FKs to tasks(id); without this a task
		// referenced by a cross-project dep can never be deleted (FK 787).
		{`DELETE FROM cross_project_deps WHERE source_task_id = ? OR target_task_id = ?`, []any{id, id}},
		{`UPDATE blockers SET task_id = NULL WHERE task_id = ?`, []any{id}},
		{`UPDATE decisions SET task_id = NULL WHERE task_id = ?`, []any{id}},
		{`UPDATE learnings SET task_id = NULL WHERE task_id = ?`, []any{id}},
		{`UPDATE tasks SET parent_id = NULL WHERE parent_id = ?`, []any{id}},
	}
	for _, s := range cleanup {
		if _, err := tx.Exec(s.query, s.args...); err != nil {
			return fmt.Errorf("delete task cleanup: %w", err)
		}
	}

	res, err := tx.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func CreateDependency(db *sql.DB, fromID, toID, depType, reason string) error {
	if depType == "" {
		depType = "blocks"
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO dependencies (from_task_id, to_task_id, dep_type, reason) VALUES (?, ?, ?, ?)`,
		fromID, toID, depType, reason)
	return err
}

func DeleteDependency(db *sql.DB, fromID, toID string) error {
	_, err := db.Exec(`DELETE FROM dependencies WHERE from_task_id = ? AND to_task_id = ?`, fromID, toID)
	return err
}

func GetBlockers(db *sql.DB, taskID string) ([]models.Dependency, error) {
	rows, err := db.Query(`SELECT from_task_id, to_task_id, dep_type, reason FROM dependencies WHERE to_task_id = ? AND dep_type = 'blocks'`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []models.Dependency
	for rows.Next() {
		var d models.Dependency
		if err := rows.Scan(&d.FromTaskID, &d.ToTaskID, &d.DepType, &d.Reason); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

func GetActiveBlockers(db *sql.DB, taskID string) ([]models.Dependency, error) {
	rows, err := db.Query(`
		SELECT d.from_task_id, d.to_task_id, d.dep_type, d.reason
		FROM dependencies d
		JOIN tasks t ON d.from_task_id = t.id
		WHERE d.to_task_id = ? AND d.dep_type = 'blocks' AND t.status NOT IN ('done','cancelled')`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []models.Dependency
	for rows.Next() {
		var d models.Dependency
		if err := rows.Scan(&d.FromTaskID, &d.ToTaskID, &d.DepType, &d.Reason); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

func SuggestNext(db *sql.DB, projectID string) (*models.Task, error) {
	// Find highest priority task that is todo, not blocked, with no pending blockers
	row := db.QueryRow(`
		SELECT t.id FROM tasks t
		WHERE t.project_id = ? AND t.status = 'todo'
		AND NOT EXISTS (
			SELECT 1 FROM dependencies d
			JOIN tasks bt ON d.from_task_id = bt.id
			WHERE d.to_task_id = t.id AND d.dep_type = 'blocks' AND bt.status NOT IN ('done','cancelled')
		)
		ORDER BY CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 END, t.sort_order, t.seq
		LIMIT 1`, projectID)

	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return GetTask(db, id)
}

const taskSelect = `SELECT t.id, t.project_id, t.seq, t.title, t.description, t.status, t.priority, t.type, t.estimate_size, t.estimate_hours, t.estimate_agent_minutes, t.actual_hours, t.parent_id, t.sort_order, t.source_type, t.agent_context, t.tags, t.start_date, t.due_date, t.created_at, t.updated_at, t.completed_at, t.is_rework, t.completion_note FROM tasks t JOIN projects p ON t.project_id = p.id`

func scanTask(row *sql.Row) (*models.Task, error) {
	var t models.Task
	var parentID, startDate, dueDate, completedAt, estimateSize, description, sourceType, agentContext, tags, completionNote sql.NullString
	var estimateHours, actualHours sql.NullFloat64
	var estimateAgentMinutes sql.NullInt64
	var createdAt, updatedAt string
	var isRework int

	err := row.Scan(&t.ID, &t.ProjectID, &t.Seq, &t.Title, &description, &t.Status, &t.Priority, &t.Type, &estimateSize, &estimateHours, &estimateAgentMinutes, &actualHours, &parentID, &t.SortOrder, &sourceType, &agentContext, &tags, &startDate, &dueDate, &createdAt, &updatedAt, &completedAt, &isRework, &completionNote)
	if err != nil {
		return nil, err
	}

	t.Description = description.String
	t.EstimateSize = estimateSize.String
	t.EstimateHours = estimateHours.Float64
	t.EstimateAgentMinutes = int(estimateAgentMinutes.Int64)
	t.ActualHours = actualHours.Float64
	t.SourceType = sourceType.String
	t.AgentContext = agentContext.String
	t.Tags = tags.String
	if parentID.Valid {
		t.ParentID = &parentID.String
	}
	if startDate.Valid {
		t.StartDate = &startDate.String
	}
	if dueDate.Valid {
		t.DueDate = &dueDate.String
	}
	if completionNote.Valid {
		t.CompletionNote = &completionNote.String
	}
	t.CreatedAt, _ = parseTime(createdAt)
	t.UpdatedAt, _ = parseTime(updatedAt)
	if completedAt.Valid {
		ct, _ := parseTime(completedAt.String)
		t.CompletedAt = &ct
	}
	t.IsRework = isRework == 1
	return &t, nil
}

func scanTaskRows(rows *sql.Rows) (*models.Task, error) {
	var t models.Task
	var parentID, startDate, dueDate, completedAt, estimateSize, description, sourceType, agentContext, tags, completionNote sql.NullString
	var estimateHours, actualHours sql.NullFloat64
	var estimateAgentMinutes sql.NullInt64
	var createdAt, updatedAt string
	var isRework int

	err := rows.Scan(&t.ID, &t.ProjectID, &t.Seq, &t.Title, &description, &t.Status, &t.Priority, &t.Type, &estimateSize, &estimateHours, &estimateAgentMinutes, &actualHours, &parentID, &t.SortOrder, &sourceType, &agentContext, &tags, &startDate, &dueDate, &createdAt, &updatedAt, &completedAt, &isRework, &completionNote)
	if err != nil {
		return nil, err
	}

	t.Description = description.String
	t.EstimateSize = estimateSize.String
	t.EstimateHours = estimateHours.Float64
	t.EstimateAgentMinutes = int(estimateAgentMinutes.Int64)
	t.ActualHours = actualHours.Float64
	t.SourceType = sourceType.String
	t.AgentContext = agentContext.String
	t.Tags = tags.String
	if parentID.Valid {
		t.ParentID = &parentID.String
	}
	if startDate.Valid {
		t.StartDate = &startDate.String
	}
	if dueDate.Valid {
		t.DueDate = &dueDate.String
	}
	if completionNote.Valid {
		t.CompletionNote = &completionNote.String
	}
	t.CreatedAt, _ = parseTime(createdAt)
	t.UpdatedAt, _ = parseTime(updatedAt)
	if completedAt.Valid {
		ct, _ := parseTime(completedAt.String)
		t.CompletedAt = &ct
	}
	t.IsRework = isRework == 1
	return &t, nil
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
