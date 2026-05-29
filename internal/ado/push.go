package ado

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
)

type PushStats struct {
	Pushed  int
	Skipped int
	Failed  int
}

func Push(conn *sql.DB, cfg *Config, teamFilter string, dryRun bool) (*PushStats, error) {
	pat, err := cfg.PAT()
	if err != nil {
		return nil, err
	}

	client := NewClient(cfg.Org, pat)
	stats := &PushStats{}

	var syncErrs []string
	for _, sync := range cfg.Syncs {
		if teamFilter != "" && !strings.EqualFold(sync.TrackProject, teamFilter) {
			continue
		}

		if err := pushTeam(conn, client, cfg, sync, stats, dryRun); err != nil {
			fmt.Printf("  warning: push %s failed: %v\n", sync.TrackProject, err)
			syncErrs = append(syncErrs, fmt.Sprintf("%s: %v", sync.TrackProject, err))
			continue
		}
	}

	if len(syncErrs) > 0 {
		return stats, fmt.Errorf("%d team push(es) failed: %s", len(syncErrs), strings.Join(syncErrs, "; "))
	}
	return stats, nil
}

func pushTeam(conn *sql.DB, client *Client, cfg *Config, sync SyncConfig, stats *PushStats, dryRun bool) error {
	project, err := ensureProject(conn, sync.TrackProject, sync.Team, true)
	if err != nil {
		return err
	}
	if project == nil {
		return nil
	}

	index := db.LoadAdoTaskIndex(conn, project.ID)

	for _, task := range index {
		if !isLocalDirty(task) {
			continue
		}

		var ctx AgentContext
		if err := json.Unmarshal([]byte(task.AgentContext), &ctx); err != nil {
			stats.Failed++
			continue
		}

		// Always push local title/description (track is the editing surface here);
		// push state only when it maps to an ADO state AND has actually changed.
		ops := []PatchOperation{
			{Op: "replace", Path: "/fields/System.Title", Value: task.Title},
			{Op: "replace", Path: "/fields/System.Description", Value: task.Description},
		}
		adoState, stateMapped := MapStatusToState(task.Status)
		stateChanged := stateMapped && storedAdoState(ctx) != adoState
		if stateChanged {
			ops = append(ops, PatchOperation{Op: "replace", Path: "/fields/System.State", Value: adoState})
		}

		if dryRun {
			fmt.Printf("[dry-run] Push %d: title/description%s\n", ctx.AdoID,
				func() string {
					if stateChanged {
						return fmt.Sprintf(" + state %s → %s", task.Status, adoState)
					}
					return ""
				}())
			stats.Pushed++
			continue
		}

		result, err := client.UpdateWorkItem(sync.Project, ctx.AdoID, ops)
		if err != nil {
			fmt.Printf("  warning: push %d failed: %v\n", ctx.AdoID, err)
			stats.Failed++
			continue
		}

		ctx.AdoRev = result.Rev
		if stateChanged {
			ctx.AdoState = adoState
		}
		ctx.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
		ctxJSON, _ := json.Marshal(ctx)
		if err := db.SyncAgentContext(conn, task.ID, string(ctxJSON)); err != nil {
			// The work-item PATCH succeeded but the local agent_context didn't
			// sync, so the task stays locally dirty and will re-push every run.
			// Count it as a failure so this silent partial-failure is visible
			// rather than masquerading as a clean push.
			fmt.Printf("  warning: update context for %s: %v\n", task.ID, err)
			stats.Failed++
			continue
		}

		stats.Pushed++
	}

	return nil
}

// storedAdoState returns the raw ADO state recorded at the last pull/push for
// this task. Pull writes AdoState=<current ADO state>; push writes the state it
// just set. When the desired state already matches, the state PATCH is skipped
// (e.g. a task that's only locally dirty in non-status fields). Empty (legacy
// rows synced before this field existed) means "unknown" → push to be safe.
func storedAdoState(ctx AgentContext) string {
	return ctx.AdoState
}
