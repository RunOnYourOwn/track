package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/RunOnYourOwn/track/internal/models"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(velocityCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(snapshotCmd)

	statusCmd.Flags().String("project", "", "Project prefix for detailed view")
	velocityCmd.Flags().String("project", "", "Project prefix (required)")
	velocityCmd.Flags().Int("weeks", 4, "Number of weeks to look back")
	healthCmd.Flags().String("project", "", "Project prefix (required)")
	snapshotCmd.Flags().String("project", "", "Project prefix (required)")
}

// ---------------------------------------------------------------------------
// Shared types for JSON output
// ---------------------------------------------------------------------------

type projectStats struct {
	Project     models.Project `json:"project"`
	Total       int            `json:"total"`
	Done        int            `json:"done"`
	InProgress  int            `json:"in_progress"`
	Todo        int            `json:"todo"`
	Blocked     int            `json:"blocked"`
	HealthScore int            `json:"health_score"`
}

// Health scoring lives in internal/db (db.ComputeHealth / db.HealthFactors) so
// the CLI report and the web dashboard share one implementation.

func healthDots(score int) string {
	// 5 dots; each dot represents 20 points
	filled := (score + 10) / 20 // round to nearest dot
	if filled > 5 {
		filled = 5
	}
	return strings.Repeat("●", filled) + strings.Repeat("○", 5-filled)
}

// ---------------------------------------------------------------------------
// track status
// ---------------------------------------------------------------------------

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Cross-project dashboard or single-project detail",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		projectPrefix, _ := cmd.Flags().GetString("project")

		today := time.Now().Format("2006-01-02")

		if projectPrefix == "" {
			// Cross-project view
			projects, err := db.ListProjects(conn)
			if err != nil {
				return err
			}

			var rows []projectStats
			for _, proj := range projects {
				tasks, err := db.ListTasks(conn, db.ListTaskOpts{ProjectID: proj.ID})
				if err != nil {
					return err
				}
				var done, wip, todo, blocked int
				for _, t := range tasks {
					switch t.Status {
					case "done":
						done++
					case "in_progress":
						wip++
					case "todo":
						todo++
					case "blocked":
						blocked++
					}
				}
				score, _ := db.ComputeHealth(&proj, tasks, proj.Prefix)
				rows = append(rows, projectStats{
					Project:     proj,
					Total:       len(tasks),
					Done:        done,
					InProgress:  wip,
					Todo:        todo,
					Blocked:     blocked,
					HealthScore: score,
				})
			}

			if jsonOutput {
				return json.NewEncoder(os.Stdout).Encode(rows)
			}

			fmt.Printf("## Status — %s\n\n", today)
			if len(rows) == 0 {
				fmt.Println("No projects found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PROJECT\tPHASE\tTOTAL\tDONE\tWIP\tTODO\tBLOCKED\tHEALTH")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n",
					r.Project.Prefix,
					r.Project.Phase,
					r.Total,
					r.Done,
					r.InProgress,
					r.Todo,
					r.Blocked,
					healthDots(r.HealthScore),
				)
			}
			return w.Flush()
		}

		// Single-project detail view
		proj, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		allTasks, err := db.ListTasks(conn, db.ListTaskOpts{ProjectID: proj.ID})
		if err != nil {
			return err
		}

		score, _ := db.ComputeHealth(proj, allTasks, proj.Prefix)

		if jsonOutput {
			type detailOut struct {
				Project     models.Project `json:"project"`
				Tasks       []models.Task  `json:"tasks"`
				HealthScore int            `json:"health_score"`
			}
			return json.NewEncoder(os.Stdout).Encode(detailOut{Project: *proj, Tasks: allTasks, HealthScore: score})
		}

		fmt.Printf("## %s — %s (%s)\n\n", proj.Prefix, proj.Name, proj.Phase)

		groups := map[string][]models.Task{
			"in_progress": {},
			"todo":        {},
			"blocked":     {},
			"done":        {},
		}
		for _, t := range allTasks {
			if _, ok := groups[t.Status]; ok {
				groups[t.Status] = append(groups[t.Status], t)
			}
		}

		printDetailGroup := func(label string, tasks []models.Task) {
			if len(tasks) == 0 {
				return
			}
			fmt.Printf("%s (%d):\n", label, len(tasks))
			for _, t := range tasks {
				displayID := t.DisplayID(proj.Prefix)
				pri := "[med]"
				if len(t.Priority) >= 3 {
					pri = fmt.Sprintf("[%s]", t.Priority[:3])
				}
				est := t.EstimateSize
				if est == "" {
					est = " "
				}
				title := t.Title
				if len(title) > 45 {
					title = title[:42] + "..."
				}

				switch t.Status {
				case "in_progress":
					fmt.Printf("  %-7s %-6s %-46s %-3s %.1fh/%.1fh\n",
						displayID, pri, title, est, t.ActualHours, t.EstimateHours)
				case "todo":
					fmt.Printf("  %-7s %-6s %-46s %-3s %.1fh\n",
						displayID, pri, title, est, t.EstimateHours)
				case "blocked":
					fmt.Printf("  %-7s %-6s %s\n", displayID, pri, title)
				case "done":
					check := "✓"
					fmt.Printf("  %-7s %s  %s  %.1fh\n", displayID, title, check, t.ActualHours)
				}
			}
			fmt.Println()
		}

		printDetailGroup("In Progress", groups["in_progress"])
		printDetailGroup("Todo — Ready", groups["todo"])
		printDetailGroup("Blocked", groups["blocked"])

		// Show only recent 5 done
		dones := groups["done"]
		if len(dones) > 5 {
			dones = dones[len(dones)-5:]
		}
		printDetailGroup("Done (recent 5)", dones)

		fmt.Printf("Health: %s  (%d/100)\n", healthDots(score), score)
		return nil
	},
}

// ---------------------------------------------------------------------------
// track velocity
// ---------------------------------------------------------------------------

type weekBucket struct {
	Label    string  `json:"week"`
	Done     int     `json:"done"`
	EstHours float64 `json:"est_hours"`
	ActHours float64 `json:"act_hours"`
	Accuracy float64 `json:"accuracy_pct"`
}

var velocityCmd = &cobra.Command{
	Use:   "velocity",
	Short: "Throughput and estimation accuracy over N weeks",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		projectPrefix, _ := cmd.Flags().GetString("project")
		weeks, _ := cmd.Flags().GetInt("weeks")

		if projectPrefix == "" {
			return fmt.Errorf("--project is required")
		}
		proj, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		cutoff := time.Now().AddDate(0, 0, -weeks*7)

		// Query done tasks completed within the window
		rows, err := conn.Query(`
			SELECT seq, title, estimate_hours, actual_hours, completed_at
			FROM tasks
			WHERE project_id = ? AND status = 'done' AND completed_at IS NOT NULL
			ORDER BY completed_at ASC`, proj.ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		buckets := map[string]*weekBucket{}
		var bucketOrder []string

		for rows.Next() {
			var seq int
			var title, completedAtStr string
			var estH, actH float64
			if err := rows.Scan(&seq, &title, &estH, &actH, &completedAtStr); err != nil {
				continue
			}
			completedAt, err := time.Parse(time.RFC3339, completedAtStr)
			if err != nil {
				continue
			}
			if completedAt.Before(cutoff) {
				continue
			}
			year, week := completedAt.ISOWeek()
			label := fmt.Sprintf("%d-W%02d", year, week)
			if _, exists := buckets[label]; !exists {
				buckets[label] = &weekBucket{Label: label}
				bucketOrder = append(bucketOrder, label)
			}
			b := buckets[label]
			b.Done++
			b.EstHours += estH
			b.ActHours += actH
		}
		if err := rows.Err(); err != nil {
			return err
		}

		// Compute accuracy per bucket
		for _, b := range buckets {
			if b.EstHours > 0 && b.ActHours > 0 {
				b.Accuracy = math.Min(b.EstHours, b.ActHours) / math.Max(b.EstHours, b.ActHours) * 100
			}
		}

		// Build ordered slice
		var result []weekBucket
		for _, label := range bucketOrder {
			result = append(result, *buckets[label])
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(result)
		}

		fmt.Printf("## Velocity — %s (%d weeks)\n\n", proj.Prefix, weeks)

		if len(result) == 0 {
			fmt.Println("No completed tasks in this period.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "WEEK\tDONE\tHOURS(est)\tHOURS(act)\tACCURACY")
		var totalDone int
		var totalEst, totalAct, totalAcc float64
		var accCount int
		for _, b := range result {
			accStr := "—"
			if b.EstHours > 0 && b.ActHours > 0 {
				accStr = fmt.Sprintf("%.0f%%", b.Accuracy)
				totalAcc += b.Accuracy
				accCount++
			}
			fmt.Fprintf(w, "%s\t%d\t%.1f\t%.1f\t%s\n",
				b.Label, b.Done, b.EstHours, b.ActHours, accStr)
			totalDone += b.Done
			totalEst += b.EstHours
			totalAct += b.ActHours
		}

		n := float64(len(result))
		avgAcc := "—"
		if accCount > 0 {
			avgAcc = fmt.Sprintf("%.0f%%", totalAcc/float64(accCount))
		}
		fmt.Fprintf(w, "Average:\t%.1f\t%.1f\t%.1f\t%s\n",
			float64(totalDone)/n, totalEst/n, totalAct/n, avgAcc)
		return w.Flush()
	},
}

// ---------------------------------------------------------------------------
// track health
// ---------------------------------------------------------------------------

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Health score breakdown for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		projectPrefix, _ := cmd.Flags().GetString("project")

		if projectPrefix == "" {
			return fmt.Errorf("--project is required")
		}
		proj, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		tasks, err := db.ListTasks(conn, db.ListTaskOpts{ProjectID: proj.ID})
		if err != nil {
			return err
		}

		score, f := db.ComputeHealth(proj, tasks, proj.Prefix)

		if jsonOutput {
			type jsonHealth struct {
				Score   int              `json:"score"`
				Factors db.HealthFactors `json:"factors"`
			}
			return json.NewEncoder(os.Stdout).Encode(jsonHealth{Score: score, Factors: f})
		}

		fmt.Printf("## Health — %s\n\n", proj.Prefix)
		fmt.Printf("Score: %d/100\n\n", score)
		fmt.Println("Factors:")

		checkMark := func(ok bool) string {
			if ok {
				return "✓"
			}
			return "✗"
		}

		// Blocker-free (+20)
		blockerDetail := ""
		blockerScore := 0
		if f.BlockerFree {
			blockerScore = 20
		}
		fmt.Printf("  Blocker-free:     %s  (+%d)%s\n", checkMark(f.BlockerFree), blockerScore, blockerDetail)

		// WIP under limit (+20)
		wipScore := 0
		if f.WIPOk {
			wipScore = 20
		}
		fmt.Printf("  WIP under limit:  %s  (+%d)  %d/%d\n", checkMark(f.WIPOk), wipScore, f.WIPCurrent, f.WIPLimit)

		// Making progress (+20)
		progressScore := 0
		if f.MakingProgress {
			progressScore = 20
		}
		progressDetail := fmt.Sprintf("  %d done this week", f.DoneThisWeek)
		fmt.Printf("  Making progress:  %s  (+%d)%s\n", checkMark(f.MakingProgress), progressScore, progressDetail)

		// No stale tasks (+20)
		staleScore := 0
		if f.NoStale {
			staleScore = 20
		}
		staleDetail := ""
		if !f.NoStale && len(f.StaleTaskIDs) > 0 {
			staleDetail = "  " + f.StaleTaskIDs[0]
			if len(f.StaleTaskIDs) > 1 {
				staleDetail += fmt.Sprintf(" (+%d more)", len(f.StaleTaskIDs)-1)
			}
		}
		fmt.Printf("  No stale tasks:   %s  (-%d)%s\n", checkMark(f.NoStale), 20-staleScore, staleDetail)

		// Estimation accuracy (partial)
		accMark := "~"
		if f.EstAccuracy {
			accMark = "✓"
		} else if f.AccuracyPct == 0 {
			accMark = "—"
		}
		accScore := int(f.PartialAccuracy)
		accDetail := ""
		if f.AccuracyPct > 0 {
			accDetail = fmt.Sprintf("  %.0f%% (target: >85%%)", f.AccuracyPct)
		} else {
			accDetail = "  no data"
		}
		fmt.Printf("  Est. accuracy:    %s  (+%d)%s\n", accMark, accScore, accDetail)

		return nil
	},
}

// ---------------------------------------------------------------------------
// track snapshot
// ---------------------------------------------------------------------------

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Record point-in-time metrics snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, _ := db.Open()
		projectPrefix, _ := cmd.Flags().GetString("project")

		if projectPrefix == "" {
			return fmt.Errorf("--project is required")
		}
		proj, err := db.GetProjectByPrefix(conn, projectPrefix)
		if err != nil {
			return fmt.Errorf("project %q not found", projectPrefix)
		}

		tasks, err := db.ListTasks(conn, db.ListTaskOpts{ProjectID: proj.ID})
		if err != nil {
			return err
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

		score, _ := db.ComputeHealth(proj, tasks, proj.Prefix)
		healthScore := float64(score)

		flowEfficiency, err := db.ComputeFlowEfficiency(conn, proj.ID)
		if err != nil {
			return fmt.Errorf("compute flow efficiency: %w", err)
		}
		var reworkRate float64
		if total > 0 {
			reworkRate = float64(rework) / float64(total)
		}

		snap := models.Snapshot{
			ID:             db.NewID(),
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
			HealthScore:    healthScore,
		}

		_, err = conn.Exec(`INSERT INTO snapshots (id, project_id, taken_at, total, done, in_progress, todo, blocked, hours_done, hours_remaining, flow_efficiency, rework_rate, health_score) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snap.ID, snap.ProjectID, snap.TakenAt.Format(time.RFC3339),
			snap.Total, snap.Done, snap.InProgress, snap.Todo, snap.Blocked,
			snap.HoursDone, snap.HoursRemaining,
			snap.FlowEfficiency, snap.ReworkRate, snap.HealthScore,
		)
		if err != nil {
			return fmt.Errorf("insert snapshot: %w", err)
		}

		if jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(snap)
		}

		fmt.Printf("Snapshot recorded: %d total, %d done, %d wip, %d todo, %d blocked (health: %d)\n",
			snap.Total, snap.Done, snap.InProgress, snap.Todo, snap.Blocked, score)
		return nil
	},
}
