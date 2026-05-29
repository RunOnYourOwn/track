package db

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

// VelocityWeek is one ISO-week bucket of completed-task throughput and estimate
// accuracy. JSON tags match the prior cmd-layer output so `--json` is unchanged.
type VelocityWeek struct {
	Label    string  `json:"week"`
	Done     int     `json:"done"`
	EstHours float64 `json:"est_hours"`
	ActHours float64 `json:"act_hours"`
	Accuracy float64 `json:"accuracy_pct"`
}

// ComputeVelocity buckets a project's done tasks completed within the last
// `weeks` weeks by ISO week, summing estimate/actual hours and computing a
// per-week accuracy = min(est,act)/max(est,act)*100. Buckets are oldest-first.
func ComputeVelocity(conn *sql.DB, projectID string, weeks int) ([]VelocityWeek, error) {
	cutoff := time.Now().AddDate(0, 0, -weeks*7)
	rows, err := conn.Query(`
		SELECT estimate_hours, actual_hours, completed_at
		FROM tasks
		WHERE project_id = ? AND status = 'done' AND completed_at IS NOT NULL
		ORDER BY completed_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := map[string]*VelocityWeek{}
	var order []string
	for rows.Next() {
		var estH, actH float64
		var completedAtStr string
		if err := rows.Scan(&estH, &actH, &completedAtStr); err != nil {
			return nil, err
		}
		completedAt, err := time.Parse(time.RFC3339, completedAtStr)
		if err != nil {
			continue // skip an unparseable timestamp rather than fail the report
		}
		if completedAt.Before(cutoff) {
			continue
		}
		year, week := completedAt.ISOWeek()
		label := fmt.Sprintf("%d-W%02d", year, week)
		b, ok := buckets[label]
		if !ok {
			b = &VelocityWeek{Label: label}
			buckets[label] = b
			order = append(order, label)
		}
		b.Done++
		b.EstHours += estH
		b.ActHours += actH
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]VelocityWeek, 0, len(order))
	for _, label := range order {
		b := buckets[label]
		if b.EstHours > 0 && b.ActHours > 0 {
			b.Accuracy = math.Min(b.EstHours, b.ActHours) / math.Max(b.EstHours, b.ActHours) * 100
		}
		result = append(result, *b)
	}
	return result, nil
}

// RecordSnapshot computes a point-in-time metrics snapshot for a project from its
// current tasks (status counts, hours done/remaining, rework rate, flow
// efficiency, health score), inserts it, and returns it.
func RecordSnapshot(conn *sql.DB, proj *models.Project) (*models.Snapshot, error) {
	tasks, err := ListTasks(conn, ListTaskOpts{ProjectID: proj.ID})
	if err != nil {
		return nil, err
	}

	var total, done, inProgress, todo, blocked, rework int
	var hoursDone, hoursRemaining float64
	for _, t := range tasks {
		total++
		if t.IsRework {
			rework++
		}
		switch t.Status {
		case "done":
			done++
			hoursDone += t.ActualHours
		case "in_progress":
			inProgress++
			hoursRemaining += t.EstimateHours
		case "todo":
			todo++
			hoursRemaining += t.EstimateHours
		case "blocked":
			blocked++
			hoursRemaining += t.EstimateHours
		}
	}

	score, _ := ComputeHealth(proj, tasks, proj.Prefix)
	flowEfficiency, err := ComputeFlowEfficiency(conn, proj.ID)
	if err != nil {
		return nil, fmt.Errorf("compute flow efficiency: %w", err)
	}
	var reworkRate float64
	if total > 0 {
		reworkRate = float64(rework) / float64(total)
	}

	snap := models.Snapshot{
		ID:             NewID(),
		ProjectID:      proj.ID,
		TakenAt:        time.Now().UTC(),
		Total:          total,
		Done:           done,
		InProgress:     inProgress,
		Todo:           todo,
		Blocked:        blocked,
		HoursDone:      hoursDone,
		HoursRemaining: hoursRemaining,
		FlowEfficiency: flowEfficiency,
		ReworkRate:     reworkRate,
		HealthScore:    float64(score),
	}

	if _, err := conn.Exec(`INSERT INTO snapshots (id, project_id, taken_at, total, done, in_progress, todo, blocked, hours_done, hours_remaining, flow_efficiency, rework_rate, health_score) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.ProjectID, snap.TakenAt.Format(time.RFC3339),
		snap.Total, snap.Done, snap.InProgress, snap.Todo, snap.Blocked,
		snap.HoursDone, snap.HoursRemaining,
		snap.FlowEfficiency, snap.ReworkRate, snap.HealthScore,
	); err != nil {
		return nil, fmt.Errorf("insert snapshot: %w", err)
	}
	return &snap, nil
}
