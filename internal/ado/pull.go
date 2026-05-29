package ado

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
)

type PullStats struct {
	Created   int
	Updated   int
	Unchanged int
	Skipped   int
	Failed    int
}

type AgentContext struct {
	AdoID        int    `json:"ado_id"`
	AdoRev       int    `json:"ado_rev"`
	AdoOrg       string `json:"ado_org"`
	AdoProject   string `json:"ado_project"`
	AdoArea      string `json:"ado_area"`
	AdoIteration string `json:"ado_iteration"`
	AdoAssigned  string `json:"ado_assigned"`
	AdoURL       string `json:"ado_url"`
	AdoStartDate string `json:"ado_start_date,omitempty"`
	AdoState     string `json:"ado_state,omitempty"` // last-synced raw ADO state, for push dedup
	LastSyncedAt string `json:"last_synced_at"`
}

func Pull(conn *sql.DB, cfg *Config, teamFilter string, dryRun bool) (*PullStats, error) {
	pat, err := cfg.PAT()
	if err != nil {
		return nil, err
	}

	client := NewClient(cfg.Org, pat)
	stats := &PullStats{}

	var syncErrs []string
	for _, sync := range cfg.Syncs {
		if teamFilter != "" && !strings.EqualFold(sync.TrackProject, teamFilter) {
			continue
		}

		if err := pullTeam(conn, client, cfg, sync, stats, dryRun); err != nil {
			fmt.Printf("  warning: sync %s failed: %v\n", sync.TrackProject, err)
			syncErrs = append(syncErrs, fmt.Sprintf("%s: %v", sync.TrackProject, err))
			continue
		}
	}

	if len(syncErrs) > 0 {
		return stats, fmt.Errorf("%d team sync(s) failed: %s", len(syncErrs), strings.Join(syncErrs, "; "))
	}
	return stats, nil
}

func pullTeam(conn *sql.DB, client *Client, cfg *Config, sync SyncConfig, stats *PullStats, dryRun bool) error {
	// Build WIQL with optional assigned_to filter
	assignedFilter := ""
	assignedTo := sync.AssignedTo
	if assignedTo == "me" || assignedTo == "@me" {
		assignedTo = cfg.Email
	}
	if assignedTo != "" {
		assignedFilter = fmt.Sprintf(" AND [System.AssignedTo] = '%s'", WiqlEscape(assignedTo))
	}

	query := fmt.Sprintf(
		"SELECT [System.Id], [System.Title], [System.State], [System.AreaPath] "+
			"FROM WorkItems WHERE [System.TeamProject] = '%s' "+
			"AND [System.State] <> 'Removed'%s "+
			"ORDER BY [System.ChangedDate] DESC",
		WiqlEscape(sync.Project), assignedFilter,
	)

	// The client percent-encodes path segments; pass the raw team name.
	wiqlResult, err := client.RunWIQL(sync.Project, sync.Team, query, 200)
	if err != nil {
		return fmt.Errorf("WIQL: %w", err)
	}

	if len(wiqlResult.WorkItems) == 0 {
		return nil
	}

	ids := make([]int, len(wiqlResult.WorkItems))
	for i, wi := range wiqlResult.WorkItems {
		ids[i] = wi.ID
	}

	workItems, err := client.GetWorkItems(sync.Project, ids)
	if err != nil {
		return fmt.Errorf("fetch details: %w", err)
	}

	// Ensure track project exists
	project, err := ensureProject(conn, sync.TrackProject, sync.Team, dryRun)
	if err != nil {
		return fmt.Errorf("ensure project: %w", err)
	}
	if dryRun && project == nil {
		fmt.Printf("[dry-run] Would create project %s (%s)\n", sync.TrackProject, sync.Team)
	}

	// Pre-load existing ADO tasks for this project (single query instead of N)
	var existingTasks map[int]*db.TaskRecord
	if project != nil {
		var idxErr error
		existingTasks, idxErr = db.LoadAdoTaskIndex(conn, project.ID)
		if idxErr != nil {
			return fmt.Errorf("load ado task index: %w", idxErr)
		}
	}

	// First pass: upsert all items
	for _, wi := range workItems {
		state := FieldString(wi.Fields, "System.State")
		if ShouldSkipState(state) {
			stats.Skipped++
			continue
		}

		result, err := upsertWorkItem(conn, cfg, sync, project, wi, existingTasks, dryRun)
		if err != nil {
			fmt.Printf("  warning: item %d: %v\n", wi.ID, err)
			stats.Failed++
			continue
		}

		switch result {
		case "created":
			stats.Created++
		case "updated":
			stats.Updated++
		case "unchanged":
			stats.Unchanged++
		case "dirty":
			stats.Skipped++
		}
	}

	// Second pass: resolve parent links
	if !dryRun {
		adoIDToLocalID, idxErr := buildAdoIndex(conn, project)
		if idxErr != nil {
			return fmt.Errorf("build ado id index: %w", idxErr)
		}
		stats.Failed += resolveParents(conn, workItems, adoIDToLocalID)
	}

	return nil
}

func upsertWorkItem(conn *sql.DB, cfg *Config, sync SyncConfig, project *db.ProjectInfo, wi WorkItem, existingTasks map[int]*db.TaskRecord, dryRun bool) (string, error) {
	title := FieldString(wi.Fields, "System.Title")
	description := StripHTML(FieldString(wi.Fields, "System.Description"))
	state := FieldString(wi.Fields, "System.State")
	wiType := FieldString(wi.Fields, "System.WorkItemType")
	areaPath := FieldString(wi.Fields, "System.AreaPath")
	iteration := FieldString(wi.Fields, "System.IterationPath")
	assignedTo := ExtractEmail(wi.Fields["System.AssignedTo"])
	targetDate := FieldString(wi.Fields, "Microsoft.VSTS.Scheduling.TargetDate")
	startDate := FieldString(wi.Fields, "Microsoft.VSTS.Scheduling.StartDate")

	trackStatus := MapStateToStatus(state)
	trackType := MapWorkItemType(wiType)

	now := time.Now().UTC().Format(time.RFC3339)

	ctx := AgentContext{
		AdoID:        wi.ID,
		AdoRev:       wi.Rev,
		AdoOrg:       cfg.Org,
		AdoProject:   sync.Project,
		AdoArea:      areaPath,
		AdoIteration: iteration,
		AdoAssigned:  assignedTo,
		AdoURL:       fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", cfg.Org, sync.Project, wi.ID),
		AdoState:     state,
		LastSyncedAt: now,
	}
	if startDate != "" {
		ctx.AdoStartDate = startDate[:min(10, len(startDate))]
	}

	ctxJSON, err := json.Marshal(ctx)
	if err != nil {
		return "", fmt.Errorf("marshal agent context: %w", err)
	}

	// Look up existing task from pre-loaded index
	existing := existingTasks[wi.ID]

	if existing != nil {
		if isLocalDirty(existing) {
			if dryRun {
				fmt.Printf("[dry-run] Skip %d (local dirty): %s\n", wi.ID, title)
			}
			return "dirty", nil
		}

		// Skip if ADO revision hasn't changed
		if existingRev := getStoredRev(existing); existingRev >= wi.Rev {
			return "unchanged", nil
		}

		if dryRun {
			fmt.Printf("[dry-run] Update %d: %s [%s]\n", wi.ID, title, trackStatus)
			return "updated", nil
		}

		if err := db.UpdateTaskField(conn, existing.ID, "title", title); err != nil {
			return "", fmt.Errorf("update title: %w", err)
		}
		if err := db.UpdateTaskField(conn, existing.ID, "description", description); err != nil {
			return "", fmt.Errorf("update description: %w", err)
		}
		if err := db.UpdateTaskField(conn, existing.ID, "type", trackType); err != nil {
			return "", fmt.Errorf("update type: %w", err)
		}
		if err := db.UpdateTaskField(conn, existing.ID, "agent_context", string(ctxJSON)); err != nil {
			return "", fmt.Errorf("update agent_context: %w", err)
		}
		if targetDate != "" {
			if err := db.UpdateTaskField(conn, existing.ID, "due_date", targetDate[:min(10, len(targetDate))]); err != nil {
				return "", fmt.Errorf("update due_date: %w", err)
			}
		}

		if existing.Status != trackStatus {
			if err := db.MoveTask(conn, existing.ID, trackStatus); err != nil {
				return "", fmt.Errorf("move task: %w", err)
			}
		}

		return "updated", nil
	}

	// Create new task
	if dryRun {
		fmt.Printf("[dry-run] Create %d: %s [%s → %s]\n", wi.ID, title, wiType, trackType)
		return "created", nil
	}

	var dueDate string
	if targetDate != "" {
		dueDate = targetDate[:min(10, len(targetDate))]
	}

	newTask, err := db.CreateTask(conn, db.CreateTaskOpts{
		ProjectID:    project.ID,
		Title:        title,
		Description:  description,
		Priority:     "medium",
		Type:         trackType,
		SourceType:   "ado",
		AgentContext: string(ctxJSON),
		DueDate:      dueDate,
	})
	if err != nil {
		return "", err
	}

	// Move to correct status if not todo
	if trackStatus != "todo" {
		if err := db.MoveTask(conn, newTask.ID, trackStatus); err != nil {
			return "", fmt.Errorf("move new task: %w", err)
		}
		// Update agent_context with fresh timestamp so dirty detection doesn't trip
		refreshedCtx := ctx
		refreshedCtx.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)
		if refreshedJSON, err := json.Marshal(refreshedCtx); err == nil {
			_ = db.SyncAgentContext(conn, newTask.ID, string(refreshedJSON))
		}
	}

	return "created", nil
}

type ProjectInfo = db.ProjectInfo

func ensureProject(conn *sql.DB, prefix, name string, dryRun bool) (*db.ProjectInfo, error) {
	p, err := db.GetProjectByPrefix(conn, prefix)
	if err == nil {
		return &db.ProjectInfo{ID: p.ID, Prefix: p.Prefix}, nil
	}

	if dryRun {
		return nil, nil
	}

	created, err := db.CreateProject(conn, prefix, name, "active", "build", "", "{}", 3)
	if err != nil {
		return nil, err
	}
	return &db.ProjectInfo{ID: created.ID, Prefix: created.Prefix}, nil
}

func getStoredRev(task *db.TaskRecord) int {
	if task.AgentContext == "" {
		return 0
	}
	var ctx AgentContext
	if err := json.Unmarshal([]byte(task.AgentContext), &ctx); err != nil {
		return 0
	}
	return ctx.AdoRev
}

func isLocalDirty(task *db.TaskRecord) bool {
	if task.AgentContext == "" {
		return false
	}
	var ctx AgentContext
	if err := json.Unmarshal([]byte(task.AgentContext), &ctx); err != nil {
		return false
	}
	if ctx.LastSyncedAt == "" {
		return false
	}
	syncedAt, err := time.Parse(time.RFC3339, ctx.LastSyncedAt)
	if err != nil {
		return false
	}
	return task.UpdatedAt.After(syncedAt.Add(5 * time.Second))
}

func buildAdoIndex(conn *sql.DB, project *db.ProjectInfo) (map[int]string, error) {
	if project == nil {
		return map[int]string{}, nil
	}
	return db.BuildAdoIDIndex(conn, project.ID)
}

// resolveParents links children to parents and returns the number of links that
// failed, so the caller can fold them into stats.Failed rather than only logging.
func resolveParents(conn *sql.DB, workItems []WorkItem, adoIDToLocalID map[int]string) int {
	failed := 0
	for _, wi := range workItems {
		parentAdoID := ExtractParentID(wi.Relations)
		if parentAdoID == 0 {
			continue
		}
		localParentID, ok := adoIDToLocalID[parentAdoID]
		if !ok {
			continue
		}
		localTaskID, ok := adoIDToLocalID[wi.ID]
		if !ok {
			continue
		}
		if err := db.SetParentID(conn, localTaskID, localParentID); err != nil {
			fmt.Printf("  warning: set parent for %s → %s: %v\n", localTaskID, localParentID, err)
			failed++
		}
	}
	return failed
}
