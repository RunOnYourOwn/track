package db

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

// Insights metrics, computed server-side so the UI only renders. Throughput,
// cycle time and accuracy honor the `days` window; the distribution and WIP
// snapshots are always current-state (all tasks), matching the prior UI.

type ThroughputMetric struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type CycleTimeMetric struct {
	AvgHours float64 `json:"avg_hours"`
	Count    int     `json:"count"`
	Source   string  `json:"source"` // "active" | "lead" | "" (no data)
}

type AccuracyMetric struct {
	AvgPct float64 `json:"avg_pct"`
	Count  int     `json:"count"` // 0 = no data
}

type DistributionMetric struct {
	Done       int `json:"done"`
	InProgress int `json:"in_progress"`
	Todo       int `json:"todo"`
	Blocked    int `json:"blocked"`
}

type WIPMetric struct {
	InProgress int `json:"in_progress"`
	Limit      int `json:"limit"`
}

type ProjectInsights struct {
	Prefix       string             `json:"prefix"`
	Name         string             `json:"name"`
	Throughput   ThroughputMetric   `json:"throughput"`
	CycleTime    CycleTimeMetric    `json:"cycle_time"`
	Accuracy     AccuracyMetric     `json:"accuracy"`
	Distribution DistributionMetric `json:"distribution"`
	WIP          WIPMetric          `json:"wip"`
}

const minActiveHours = 1.0 / 60.0 // 1 minute minimum to count as real active work

// ComputeInsights builds per-project analytics. days <= 0 means all time.
func ComputeInsights(db *sql.DB, days int) ([]ProjectInsights, error) {
	projects, err := ListProjects(db)
	if err != nil {
		return nil, err
	}

	// One query for every task, grouped by project — avoids a ListTasks per
	// project (the old N+1).
	allTasks, err := ListTasks(db, ListTaskOpts{})
	if err != nil {
		return nil, err
	}
	tasksByProject := make(map[string][]models.Task, len(projects))
	for _, t := range allTasks {
		tasksByProject[t.ProjectID] = append(tasksByProject[t.ProjectID], t)
	}

	var cutoff time.Time
	if days > 0 {
		cutoff = time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	}

	out := make([]ProjectInsights, 0, len(projects))
	for i := range projects {
		p := &projects[i]
		// Cancelled work is descoped — exclude it from every insight metric
		// (throughput, cycle time, accuracy, distribution, WIP).
		projectTasks := tasksByProject[p.ID]
		tasks := make([]models.Task, 0, len(projectTasks))
		for _, t := range projectTasks {
			if t.Status != "cancelled" {
				tasks = append(tasks, t)
			}
		}

		windowed := tasks
		if days > 0 {
			windowed = windowed[:0:0]
			for _, t := range tasks {
				if !taskRef(t).Before(cutoff) {
					windowed = append(windowed, t)
				}
			}
		}

		out = append(out, ProjectInsights{
			Prefix:       p.Prefix,
			Name:         p.Name,
			Throughput:   computeThroughput(windowed),
			CycleTime:    computeCycleTime(windowed),
			Accuracy:     computeAccuracy(windowed),
			Distribution: computeDistribution(tasks),
			WIP:          WIPMetric{InProgress: countInProgress(tasks), Limit: p.WIPLimit},
		})
	}
	return out, nil
}

// taskRef mirrors the UI's `completed_at || updated_at || created_at` recency key.
func taskRef(t models.Task) time.Time {
	if t.CompletedAt != nil {
		return *t.CompletedAt
	}
	if !t.UpdatedAt.IsZero() {
		return t.UpdatedAt
	}
	return t.CreatedAt
}

func computeThroughput(tasks []models.Task) ThroughputMetric {
	done := 0
	for _, t := range tasks {
		if t.Status == "done" {
			done++
		}
	}
	return ThroughputMetric{Done: done, Total: len(tasks)}
}

func computeCycleTime(tasks []models.Task) CycleTimeMetric {
	var completed []models.Task
	for _, t := range tasks {
		if t.Status == "done" && t.CompletedAt != nil && !t.CreatedAt.IsZero() {
			completed = append(completed, t)
		}
	}
	if len(completed) == 0 {
		return CycleTimeMetric{}
	}

	// Best signal: real active time logged via status history.
	var actual []float64
	for _, t := range completed {
		if t.ActualHours >= minActiveHours {
			actual = append(actual, t.ActualHours)
		}
	}
	if len(actual) > 0 {
		return CycleTimeMetric{AvgHours: mean(actual), Count: len(actual), Source: "active"}
	}

	// Fallback: lead time, but only if tasks were created individually over time.
	// If the top-2 creation hours hold >50% of tasks, it's a bulk import (lead
	// time is meaningless), so report no data.
	hourCounts := map[string]int{}
	for _, t := range completed {
		hourCounts[t.CreatedAt.Format("2006-01-02T15")]++
	}
	counts := make([]int, 0, len(hourCounts))
	for _, c := range hourCounts {
		counts = append(counts, c)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(counts)))
	topTwo := 0
	if len(counts) > 0 {
		topTwo += counts[0]
	}
	if len(counts) > 1 {
		topTwo += counts[1]
	}
	if float64(topTwo)/float64(len(completed)) > 0.5 {
		return CycleTimeMetric{}
	}

	var lead []float64
	for _, t := range completed {
		h := t.CompletedAt.Sub(t.CreatedAt).Hours()
		if h > 5.0/60.0 {
			lead = append(lead, h)
		}
	}
	if len(lead) == 0 {
		return CycleTimeMetric{}
	}
	return CycleTimeMetric{AvgHours: mean(lead), Count: len(lead), Source: "lead"}
}

func computeAccuracy(tasks []models.Task) AccuracyMetric {
	// Agent axis: actual_hours is the agent's active in_progress time, so the
	// matching estimate is estimate_agent_minutes (agent), not estimate_hours
	// (human) — comparing those two would mix axes and isn't meaningful.
	var accs []float64
	for _, t := range tasks {
		if t.EstimateAgentMinutes > 0 && t.ActualHours > 0 {
			lo, hi := float64(t.EstimateAgentMinutes)/60.0, t.ActualHours
			if lo > hi {
				lo, hi = hi, lo
			}
			accs = append(accs, lo/hi*100)
		}
	}
	if len(accs) == 0 {
		return AccuracyMetric{}
	}
	return AccuracyMetric{AvgPct: mean(accs), Count: len(accs)}
}

func computeDistribution(tasks []models.Task) DistributionMetric {
	var d DistributionMetric
	for _, t := range tasks {
		switch {
		case t.Status == "done":
			d.Done++
		case t.Status == "in_progress":
			d.InProgress++
		case t.Status == "blocked" || strings.HasPrefix(t.Status, "waiting"):
			d.Blocked++
		default:
			d.Todo++
		}
	}
	return d
}

func countInProgress(tasks []models.Task) int {
	n := 0
	for _, t := range tasks {
		if t.Status == "in_progress" {
			n++
		}
	}
	return n
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
