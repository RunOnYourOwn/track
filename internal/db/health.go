package db

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/RunOnYourOwn/track/internal/models"
)

// HealthFactors is the breakdown behind a project's health score.
type HealthFactors struct {
	BlockerFree     bool
	WIPOk           bool
	MakingProgress  bool
	NoStale         bool // no tasks stale > 14 days
	EstAccuracy     bool // > 70% estimation accuracy
	StaleTaskIDs    []string
	WIPCurrent      int
	WIPLimit        int
	DoneThisWeek    int
	AccuracyPct     float64
	PartialAccuracy float64 // 0-20 for partial scoring
}

// ComputeHealth calculates a 0-100 health score (5 factors × 20 pts each). A
// factor with partial progress (e.g. accuracy) scores proportionally. This is
// the single source of truth shared by the CLI report and the web dashboard.
func ComputeHealth(proj *models.Project, tasks []models.Task, prefix string) (int, HealthFactors) {
	var f HealthFactors
	f.WIPLimit = proj.WIPLimit
	if f.WIPLimit == 0 {
		f.WIPLimit = 3
	}

	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	staleThreshold := now.Add(-14 * 24 * time.Hour)

	var wipCount, blockedCount, doneCount int
	var estimatedPairs int

	for _, t := range tasks {
		switch {
		case t.Status == "in_progress":
			wipCount++
			if t.UpdatedAt.Before(staleThreshold) {
				f.StaleTaskIDs = append(f.StaleTaskIDs, t.DisplayID(prefix)+
					" stale "+strconv.Itoa(int(now.Sub(t.UpdatedAt).Hours()/24))+" days")
			}
		case t.Status == "blocked":
			blockedCount++
		case strings.HasPrefix(t.Status, "waiting"):
			blockedCount++
		case t.Status == "done":
			doneCount++
			if t.CompletedAt != nil && t.CompletedAt.After(weekAgo) {
				f.DoneThisWeek++
			}
			if t.EstimateHours > 0 && t.ActualHours > 0 {
				estimatedPairs++
			}
		case t.Status == "todo":
			if t.UpdatedAt.Before(staleThreshold) {
				f.StaleTaskIDs = append(f.StaleTaskIDs, t.DisplayID(prefix)+
					" stale "+strconv.Itoa(int(now.Sub(t.UpdatedAt).Hours()/24))+" days")
			}
		}
	}

	f.WIPCurrent = wipCount
	f.BlockerFree = blockedCount == 0
	f.WIPOk = wipCount <= f.WIPLimit
	f.MakingProgress = f.DoneThisWeek > 0 || doneCount > 0
	f.NoStale = len(f.StaleTaskIDs) == 0

	if estimatedPairs > 0 {
		var accuracySum float64
		for _, t := range tasks {
			if t.Status == "done" && t.EstimateHours > 0 && t.ActualHours > 0 {
				acc := math.Min(t.EstimateHours, t.ActualHours) / math.Max(t.EstimateHours, t.ActualHours)
				accuracySum += acc
			}
		}
		f.AccuracyPct = (accuracySum / float64(estimatedPairs)) * 100
	}
	f.EstAccuracy = f.AccuracyPct >= 70

	score := 0
	if f.BlockerFree {
		score += 20
	}
	if f.WIPOk {
		score += 20
	}
	if f.MakingProgress {
		score += 20
	}
	if f.NoStale {
		score += 20
	}
	if estimatedPairs > 0 {
		accScore := int(math.Min(f.AccuracyPct/100.0*20, 20))
		score += accScore
		f.PartialAccuracy = float64(accScore)
	}

	return score, f
}
