package db

import (
	"database/sql"
	"strings"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

func inClause(n int) string {
	if n == 0 {
		return "()"
	}
	return "(" + strings.Repeat("?,", n-1) + "?)"
}

func GetSessionStats(conn *sql.DB, sessionID string) (*models.SessionStats, error) {
	// Step 1: fetch session window
	var projectID, startedAtStr string
	var endedAtStr sql.NullString
	err := conn.QueryRow(
		`SELECT project_id, started_at, ended_at FROM sessions WHERE id = ?`, sessionID,
	).Scan(&projectID, &startedAtStr, &endedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	startedAt, _ := parseTime(startedAtStr)
	var endedAt time.Time
	if endedAtStr.Valid {
		endedAt, _ = parseTime(endedAtStr.String)
	} else {
		endedAt = time.Now().UTC()
	}

	startStr := startedAt.Format(time.RFC3339)
	endStr := endedAt.Format(time.RFC3339)

	// Step 2: get all task IDs for the project
	type taskInfo struct {
		id                   string
		seq                  int
		title                string
		estimateHours        float64
		estimateAgentMinutes int
		actualHours          float64
	}
	rows, err := conn.Query(`SELECT id, seq, title, COALESCE(estimate_hours, 0), COALESCE(estimate_agent_minutes, 0), COALESCE(actual_hours, 0) FROM tasks WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []taskInfo
	var taskIDs []any
	taskMap := make(map[string]taskInfo)
	for rows.Next() {
		var ti taskInfo
		if err := rows.Scan(&ti.id, &ti.seq, &ti.title, &ti.estimateHours, &ti.estimateAgentMinutes, &ti.actualHours); err != nil {
			return nil, err
		}
		tasks = append(tasks, ti)
		taskIDs = append(taskIDs, ti.id)
		taskMap[ti.id] = ti
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stats := &models.SessionStats{
		SessionID: sessionID,
		Tasks:     []models.TaskActivity{},
		Commits:   []models.TaskCommit{},
	}

	// Step 3: time logged for this session
	var totalHours sql.NullFloat64
	err = conn.QueryRow(
		`SELECT SUM(hours) FROM time_entries WHERE session_id = ?`, sessionID,
	).Scan(&totalHours)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if totalHours.Valid {
		stats.TotalHours = totalHours.Float64
	}

	if len(taskIDs) == 0 {
		return stats, nil
	}

	// Step 4: status changes during window
	query := `SELECT task_id, status, entered_at FROM task_status_history
		WHERE task_id IN ` + inClause(len(taskIDs)) + `
		AND entered_at >= ? AND entered_at <= ?
		ORDER BY entered_at`
	args := append(taskIDs, startStr, endStr)

	histRows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer histRows.Close()

	touchedSet := make(map[string]bool)
	completedSet := make(map[string]bool)
	for histRows.Next() {
		var taskID, status, enteredAtStr string
		if err := histRows.Scan(&taskID, &status, &enteredAtStr); err != nil {
			return nil, err
		}
		touchedSet[taskID] = true
		if status == "done" {
			completedSet[taskID] = true
		}
	}
	if err := histRows.Err(); err != nil {
		return nil, err
	}

	// Step 5: commits within the session window
	commitQuery := `SELECT task_id, commit_hash, repo, committed_at, message, files_changed
		FROM task_commits
		WHERE task_id IN ` + inClause(len(taskIDs)) + `
		AND committed_at >= ? AND committed_at <= ?
		ORDER BY committed_at DESC`
	commitArgs := append(taskIDs, startStr, endStr)

	commitRows, err := conn.Query(commitQuery, commitArgs...)
	if err != nil {
		return nil, err
	}
	defer commitRows.Close()

	for commitRows.Next() {
		var c models.TaskCommit
		var committedAtStr string
		if err := commitRows.Scan(&c.TaskID, &c.CommitHash, &c.Repo, &committedAtStr, &c.Message, &c.FilesChanged); err != nil {
			return nil, err
		}
		c.CommittedAt, _ = parseTime(committedAtStr)
		stats.Commits = append(stats.Commits, c)
	}
	if err := commitRows.Err(); err != nil {
		return nil, err
	}
	stats.CommitCount = len(stats.Commits)

	// Step 6: cycle time for completed tasks
	cycleTimeMap := make(map[string]int64)
	if len(completedSet) > 0 {
		completedIDs := make([]any, 0, len(completedSet))
		for id := range completedSet {
			completedIDs = append(completedIDs, id)
		}

		// Get first in_progress time for each completed task
		ipQuery := `SELECT task_id, MIN(entered_at) FROM task_status_history
			WHERE task_id IN ` + inClause(len(completedIDs)) + `
			AND status = 'in_progress'
			GROUP BY task_id`

		ipRows, err := conn.Query(ipQuery, completedIDs...)
		if err != nil {
			return nil, err
		}
		defer ipRows.Close()

		ipTimes := make(map[string]time.Time)
		for ipRows.Next() {
			var taskID, enteredAtStr string
			if err := ipRows.Scan(&taskID, &enteredAtStr); err != nil {
				return nil, err
			}
			t, _ := parseTime(enteredAtStr)
			ipTimes[taskID] = t
		}

		// Get last done time within the session window for each completed task
		doneQuery := `SELECT task_id, MAX(entered_at) FROM task_status_history
			WHERE task_id IN ` + inClause(len(completedIDs)) + `
			AND status = 'done'
			AND entered_at >= ? AND entered_at <= ?
			GROUP BY task_id`
		doneArgs := append(completedIDs, startStr, endStr)

		doneRows, err := conn.Query(doneQuery, doneArgs...)
		if err != nil {
			return nil, err
		}
		defer doneRows.Close()

		for doneRows.Next() {
			var taskID, enteredAtStr string
			if err := doneRows.Scan(&taskID, &enteredAtStr); err != nil {
				return nil, err
			}
			doneTime, _ := parseTime(enteredAtStr)
			if ipTime, ok := ipTimes[taskID]; ok {
				cycleTimeMap[taskID] = int64(doneTime.Sub(ipTime).Seconds())
			}
		}
	}

	// Build task activities
	for taskID := range touchedSet {
		ti := taskMap[taskID]
		ta := models.TaskActivity{
			TaskID:               taskID,
			Title:                ti.title,
			Seq:                  ti.seq,
			Completed:            completedSet[taskID],
			Touched:              true,
			EstimateHours:        ti.estimateHours,
			EstimateAgentMinutes: ti.estimateAgentMinutes,
			ActualHours:          ti.actualHours,
		}
		if ct, ok := cycleTimeMap[taskID]; ok {
			ta.CycleTimeSec = &ct
		}
		stats.Tasks = append(stats.Tasks, ta)
	}

	stats.TasksTouched = len(touchedSet)
	stats.TasksCompleted = len(completedSet)

	return stats, nil
}

func GetSessionStatsBatch(conn *sql.DB, sessionIDs []string) (map[string]models.SessionSummary, error) {
	result := make(map[string]models.SessionSummary, len(sessionIDs))

	for _, sid := range sessionIDs {
		stats, err := GetSessionStats(conn, sid)
		if err != nil {
			return nil, err
		}
		if stats == nil {
			result[sid] = models.SessionSummary{SessionID: sid}
			continue
		}
		result[sid] = models.SessionSummary{
			SessionID:      sid,
			TotalHours:     stats.TotalHours,
			TasksCompleted: stats.TasksCompleted,
			TasksTouched:   stats.TasksTouched,
			CommitCount:    stats.CommitCount,
		}
	}

	return result, nil
}
