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

	for _, sync := range cfg.Syncs {
		if teamFilter != "" && !strings.EqualFold(sync.TrackProject, teamFilter) {
			continue
		}

		if err := pushTeam(conn, client, cfg, sync, stats, dryRun); err != nil {
			return stats, fmt.Errorf("push %s: %w", sync.TrackProject, err)
		}
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

		adoState, ok := MapStatusToState(task.Status)
		if !ok {
			stats.Skipped++
			continue
		}

		var ctx AgentContext
		if err := json.Unmarshal([]byte(task.AgentContext), &ctx); err != nil {
			stats.Failed++
			continue
		}

		// Compare stored state with desired — skip if already matches
		currentState := storedAdoState(ctx)
		if currentState == adoState {
			stats.Skipped++
			continue
		}

		ops := []PatchOperation{
			{Op: "replace", Path: "/fields/System.State", Value: adoState},
		}

		if dryRun {
			fmt.Printf("[dry-run] Push %d: %s → %s\n", ctx.AdoID, task.Status, adoState)
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
		ctx.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
		ctxJSON, _ := json.Marshal(ctx)
		if err := db.SyncAgentContext(conn, task.ID, string(ctxJSON)); err != nil {
			fmt.Printf("  warning: update context for %s: %v\n", task.ID, err)
		}

		stats.Pushed++
	}

	return nil
}

func storedAdoState(ctx AgentContext) string {
	// The pull stores the mapped status, not the raw ADO state.
	// We don't store the literal ADO state in agent_context, so we infer from rev tracking.
	// If the rev hasn't changed remotely (tracked via pull), we can safely push.
	// For comparison purposes, we return empty — meaning "unknown, always push".
	return ""
}
