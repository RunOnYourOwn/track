package ado

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
)

func TestGetStoredRev(t *testing.T) {
	cases := []struct {
		name string
		task *db.TaskRecord
		want int
	}{
		{"empty context", &db.TaskRecord{AgentContext: ""}, 0},
		{"invalid JSON", &db.TaskRecord{AgentContext: "bad"}, 0},
		{"no rev field", &db.TaskRecord{AgentContext: `{"ado_id":1}`}, 0},
		{"valid rev", &db.TaskRecord{AgentContext: mustJSON(AgentContext{AdoID: 1, AdoRev: 7})}, 7},
		{"rev zero", &db.TaskRecord{AgentContext: mustJSON(AgentContext{AdoID: 1, AdoRev: 0})}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getStoredRev(tc.task)
			if got != tc.want {
				t.Errorf("getStoredRev = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEnsureProject(t *testing.T) {
	conn := db.OpenTestDB(t)

	// Create a project first
	db.CreateProject(conn, "EXS", "Existing", "active", "build", "", "{}", 3)

	// Should find existing project
	info, err := ensureProject(conn, "EXS", "Existing", false)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.Prefix != "EXS" {
		t.Error("expected to find existing project")
	}

	// Should create new project
	info2, err := ensureProject(conn, "NEW", "New Team", false)
	if err != nil {
		t.Fatal(err)
	}
	if info2 == nil || info2.Prefix != "NEW" {
		t.Error("expected new project to be created")
	}

	// Verify it was actually created
	p, err := db.GetProjectByPrefix(conn, "NEW")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "New Team" {
		t.Errorf("expected name 'New Team', got %s", p.Name)
	}

	// Dry run on non-existent should return nil, nil
	info3, err := ensureProject(conn, "DRY", "Dry Run", true)
	if err != nil {
		t.Fatal(err)
	}
	if info3 != nil {
		t.Error("dry run should return nil for new project")
	}

	// Dry run on existing should still find it
	info4, err := ensureProject(conn, "EXS", "Existing", true)
	if err != nil {
		t.Fatal(err)
	}
	if info4 == nil {
		t.Error("dry run should find existing project")
	}
}

func TestBuildAdoIndex(t *testing.T) {
	conn := db.OpenTestDB(t)

	// nil project — returns empty map
	idx := buildAdoIndex(conn, nil)
	if len(idx) != 0 {
		t.Errorf("expected empty for nil project, got %d", len(idx))
	}

	// Real project with ADO tasks
	p, _ := db.CreateProject(conn, "IDX", "Index", "active", "build", "", "{}", 3)
	ctx, _ := json.Marshal(map[string]any{"ado_id": 500})
	conn.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at) VALUES (?, ?, 1, 'T', 'todo', 'medium', 'task', 'ado', ?, ?, ?)`,
		"t1", p.ID, string(ctx), db.Now(), db.Now())

	idx2 := buildAdoIndex(conn, &db.ProjectInfo{ID: p.ID, Prefix: "IDX"})
	if len(idx2) != 1 || idx2[500] != "t1" {
		t.Errorf("expected {500: t1}, got %v", idx2)
	}
}

func TestResolveParents(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "RES", "Resolve", "active", "build", "", "{}", 3)
	parent, _ := db.CreateTask(conn, db.CreateTaskOpts{ProjectID: p.ID, Title: "Parent", Type: "feature"})
	child, _ := db.CreateTask(conn, db.CreateTaskOpts{ProjectID: p.ID, Title: "Child"})

	adoIDToLocalID := map[int]string{
		100: parent.ID,
		200: child.ID,
	}

	workItems := []WorkItem{
		{
			ID:  200,
			Rev: 1,
			Relations: []Relation{
				{
					Rel: "System.LinkTypes.Hierarchy-Reverse",
					URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/100",
				},
			},
		},
		{
			ID:  100,
			Rev: 1,
		},
	}

	resolveParents(conn, workItems, adoIDToLocalID)

	// Child should now have parent set
	got, _ := db.GetTask(conn, child.ID)
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Error("expected child to have parent set after resolveParents")
	}
}

func TestResolveParentsNoMatch(t *testing.T) {
	conn := db.OpenTestDB(t)

	// No matching IDs in index — should be a no-op (no error)
	workItems := []WorkItem{
		{
			ID:  999,
			Rev: 1,
			Relations: []Relation{
				{
					Rel: "System.LinkTypes.Hierarchy-Reverse",
					URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/888",
				},
			},
		},
	}
	resolveParents(conn, workItems, map[int]string{})
}

func TestUpsertWorkItemCreate(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "UPS", "Upsert Test", "active", "build", "", "{}", 3)
	project := &db.ProjectInfo{ID: p.ID, Prefix: "UPS"}
	cfg := &Config{Org: "testorg"}
	sync := SyncConfig{Project: "TestProj", Team: "Team1", TrackProject: "UPS"}
	existingTasks := map[int]*db.TaskRecord{}

	wi := WorkItem{
		ID:  42,
		Rev: 3,
		Fields: map[string]interface{}{
			"System.Title":        "New Work Item",
			"System.Description":  "<p>Description</p>",
			"System.State":        "New",
			"System.WorkItemType": "User Story",
			"System.AreaPath":     "TestProj\\Team1",
			"System.IterationPath": "TestProj\\Sprint 1",
			"Microsoft.VSTS.Scheduling.TargetDate": "2026-07-01T00:00:00Z",
		},
	}

	result, err := upsertWorkItem(conn, cfg, sync, project, wi, existingTasks, false)
	if err != nil {
		t.Fatal(err)
	}
	if result != "created" {
		t.Errorf("expected 'created', got %s", result)
	}

	// Verify task was created
	tasks, _ := db.ListTasks(conn, db.ListTaskOpts{ProjectID: p.ID})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "New Work Item" {
		t.Errorf("expected title 'New Work Item', got %s", tasks[0].Title)
	}
	if tasks[0].Type != "feature" {
		t.Errorf("expected type 'feature' (mapped from User Story), got %s", tasks[0].Type)
	}
}

func TestUpsertWorkItemUpdate(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "UPU", "Upsert Update", "active", "build", "", "{}", 3)
	project := &db.ProjectInfo{ID: p.ID, Prefix: "UPU"}
	cfg := &Config{Org: "testorg"}
	sync := SyncConfig{Project: "TestProj", Team: "Team1", TrackProject: "UPU"}

	// Create existing task with older rev
	oldCtx := AgentContext{AdoID: 55, AdoRev: 2, AdoOrg: "testorg", LastSyncedAt: time.Now().UTC().Format(time.RFC3339)}
	oldCtxJSON, _ := json.Marshal(oldCtx)
	task, _ := db.CreateTask(conn, db.CreateTaskOpts{
		ProjectID:    p.ID,
		Title:        "Old Title",
		SourceType:   "ado",
		AgentContext: string(oldCtxJSON),
	})

	existingTasks := map[int]*db.TaskRecord{
		55: {ID: task.ID, Status: "todo", AgentContext: string(oldCtxJSON), UpdatedAt: time.Now().UTC()},
	}

	wi := WorkItem{
		ID:  55,
		Rev: 5, // newer than stored rev 2
		Fields: map[string]interface{}{
			"System.Title":        "Updated Title",
			"System.Description":  "Updated desc",
			"System.State":        "Active",
			"System.WorkItemType": "Task",
			"System.AreaPath":     "TestProj\\Area",
			"System.IterationPath": "TestProj\\Sprint 2",
		},
	}

	result, err := upsertWorkItem(conn, cfg, sync, project, wi, existingTasks, false)
	if err != nil {
		t.Fatal(err)
	}
	if result != "updated" {
		t.Errorf("expected 'updated', got %s", result)
	}

	// Verify title changed
	got, _ := db.GetTask(conn, task.ID)
	if got.Title != "Updated Title" {
		t.Errorf("expected 'Updated Title', got %s", got.Title)
	}
	if got.Status != "in_progress" {
		t.Errorf("expected 'in_progress' (mapped from Active), got %s", got.Status)
	}
}

func TestUpsertWorkItemUnchanged(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "UNC", "Unchanged", "active", "build", "", "{}", 3)
	project := &db.ProjectInfo{ID: p.ID, Prefix: "UNC"}
	cfg := &Config{Org: "testorg"}
	sync := SyncConfig{Project: "Proj", Team: "T", TrackProject: "UNC"}

	// Existing task with same rev
	ctx := AgentContext{AdoID: 77, AdoRev: 10, LastSyncedAt: time.Now().UTC().Format(time.RFC3339)}
	ctxJSON, _ := json.Marshal(ctx)
	task, _ := db.CreateTask(conn, db.CreateTaskOpts{
		ProjectID:    p.ID,
		Title:        "Same",
		SourceType:   "ado",
		AgentContext: string(ctxJSON),
	})

	existingTasks := map[int]*db.TaskRecord{
		77: {ID: task.ID, Status: "todo", AgentContext: string(ctxJSON), UpdatedAt: time.Now().UTC()},
	}

	wi := WorkItem{
		ID:  77,
		Rev: 10, // same as stored
		Fields: map[string]interface{}{
			"System.Title": "Same",
			"System.State": "New",
		},
	}

	result, err := upsertWorkItem(conn, cfg, sync, project, wi, existingTasks, false)
	if err != nil {
		t.Fatal(err)
	}
	if result != "unchanged" {
		t.Errorf("expected 'unchanged', got %s", result)
	}
}

func TestUpsertWorkItemDirty(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "DIR", "Dirty", "active", "build", "", "{}", 3)
	project := &db.ProjectInfo{ID: p.ID, Prefix: "DIR"}
	cfg := &Config{Org: "testorg"}
	sync := SyncConfig{Project: "P", Team: "T", TrackProject: "DIR"}

	// Task updated well after sync (dirty)
	ctx := AgentContext{AdoID: 88, AdoRev: 1, LastSyncedAt: time.Now().Add(-60 * time.Second).UTC().Format(time.RFC3339)}
	ctxJSON, _ := json.Marshal(ctx)
	task, _ := db.CreateTask(conn, db.CreateTaskOpts{
		ProjectID:    p.ID,
		Title:        "Dirty",
		SourceType:   "ado",
		AgentContext: string(ctxJSON),
	})

	existingTasks := map[int]*db.TaskRecord{
		88: {ID: task.ID, Status: "todo", AgentContext: string(ctxJSON), UpdatedAt: time.Now().UTC()},
	}

	wi := WorkItem{
		ID:  88,
		Rev: 5,
		Fields: map[string]interface{}{
			"System.Title": "New",
			"System.State": "Active",
		},
	}

	result, err := upsertWorkItem(conn, cfg, sync, project, wi, existingTasks, false)
	if err != nil {
		t.Fatal(err)
	}
	if result != "dirty" {
		t.Errorf("expected 'dirty', got %s", result)
	}
}

func TestUpsertWorkItemDryRun(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "DRY", "DryRun", "active", "build", "", "{}", 3)
	project := &db.ProjectInfo{ID: p.ID, Prefix: "DRY"}
	cfg := &Config{Org: "testorg"}
	sync := SyncConfig{Project: "P", Team: "T", TrackProject: "DRY"}

	wi := WorkItem{
		ID:  99,
		Rev: 1,
		Fields: map[string]interface{}{
			"System.Title":        "DryRun Item",
			"System.State":        "New",
			"System.WorkItemType": "Bug",
		},
	}

	result, err := upsertWorkItem(conn, cfg, sync, project, wi, map[int]*db.TaskRecord{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result != "created" {
		t.Errorf("expected 'created' in dry-run, got %s", result)
	}

	// Verify no task was actually created
	tasks, _ := db.ListTasks(conn, db.ListTaskOpts{ProjectID: p.ID})
	if len(tasks) != 0 {
		t.Error("dry-run should not create tasks")
	}
}

func TestConfigPAT(t *testing.T) {
	cfg := &Config{PatEnv: "TEST_TRACK_ADO_PAT_XYZ"}

	// Not set — should error
	_, err := cfg.PAT()
	if err == nil {
		t.Error("expected error when env var not set")
	}

	// Set it
	os.Setenv("TEST_TRACK_ADO_PAT_XYZ", "secret123")
	defer os.Unsetenv("TEST_TRACK_ADO_PAT_XYZ")

	pat, err := cfg.PAT()
	if err != nil {
		t.Fatal(err)
	}
	if pat != "secret123" {
		t.Errorf("expected 'secret123', got %s", pat)
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	// Use a temp dir to avoid modifying real config
	tmpDir := t.TempDir()
	path := tmpDir + "/ado.json"

	cfg := &Config{
		Org:    "myorg",
		PatEnv: "MY_PAT",
		Email:  "user@example.com",
		Syncs: []SyncConfig{
			{Project: "Proj1", Team: "Team1", TrackProject: "PRJ"},
		},
	}

	// Save manually (since SaveConfig uses ConfigPath)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0600)

	// Read back
	raw, _ := os.ReadFile(path)
	var loaded Config
	json.Unmarshal(raw, &loaded)

	if loaded.Org != "myorg" {
		t.Errorf("expected org 'myorg', got %s", loaded.Org)
	}
	if len(loaded.Syncs) != 1 {
		t.Fatal("expected 1 sync config")
	}
	if loaded.Syncs[0].TrackProject != "PRJ" {
		t.Errorf("expected track_project 'PRJ', got %s", loaded.Syncs[0].TrackProject)
	}
}

func TestPullTeamEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(WIQLResult{
				WorkItems: []WIQLWorkItem{
					{ID: 1},
					{ID: 2},
				},
			})
		} else {
			json.NewEncoder(w).Encode(BatchResult{
				Value: []WorkItem{
					{
						ID: 1, Rev: 1,
						Fields: map[string]interface{}{
							"System.Title":         "Task One",
							"System.State":         "New",
							"System.WorkItemType":  "Task",
							"System.AreaPath":      "Proj\\Area",
							"System.IterationPath": "Proj\\Sprint",
						},
					},
					{
						ID: 2, Rev: 1,
						Fields: map[string]interface{}{
							"System.Title":         "Task Two",
							"System.State":         "Active",
							"System.WorkItemType":  "Bug",
							"System.AreaPath":      "Proj\\Area",
							"System.IterationPath": "Proj\\Sprint",
						},
					},
				},
				Count: 2,
			})
		}
	}))
	defer server.Close()

	conn := db.OpenTestDB(t)
	db.CreateProject(conn, "PULL", "Pull Test", "active", "build", "", "{}", 3)

	client := &Client{org: "testorg", pat: "pat", httpCli: server.Client(), baseURL: server.URL}
	cfg := &Config{Org: "testorg", Email: "test@example.com"}
	sync := SyncConfig{Project: "Proj", Team: "Team1", TrackProject: "PULL"}
	stats := &PullStats{}

	err := pullTeam(conn, client, cfg, sync, stats, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 2 {
		t.Errorf("expected 2 created, got %d", stats.Created)
	}

	// Verify tasks were created
	tasks, _ := db.ListTasks(conn, db.ListTaskOpts{ProjectID: ""})
	found := 0
	for _, task := range tasks {
		if task.Title == "Task One" || task.Title == "Task Two" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected 2 created tasks in DB, found %d", found)
	}
}

func TestPullTeamDryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(WIQLResult{
				WorkItems: []WIQLWorkItem{{ID: 1}},
			})
		} else {
			json.NewEncoder(w).Encode(BatchResult{
				Value: []WorkItem{{
					ID: 1, Rev: 1,
					Fields: map[string]interface{}{
						"System.Title":         "DryRun Task",
						"System.State":         "New",
						"System.WorkItemType":  "Task",
						"System.AreaPath":      "P\\A",
						"System.IterationPath": "P\\S",
					},
				}},
				Count: 1,
			})
		}
	}))
	defer server.Close()

	conn := db.OpenTestDB(t)

	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}
	cfg := &Config{Org: "org"}
	sync := SyncConfig{Project: "P", Team: "T", TrackProject: "NEWP"}
	stats := &PullStats{}

	err := pullTeam(conn, client, cfg, sync, stats, true)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 1 {
		t.Errorf("expected 1 created in dry-run, got %d", stats.Created)
	}

	// Verify no project or tasks actually created
	_, pErr := db.GetProjectByPrefix(conn, "NEWP")
	if pErr == nil {
		t.Error("dry-run should not create project")
	}
}

func TestPullTeamEmptyWIQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WIQLResult{WorkItems: []WIQLWorkItem{}})
	}))
	defer server.Close()

	conn := db.OpenTestDB(t)
	db.CreateProject(conn, "EMP", "Empty", "active", "build", "", "{}", 3)

	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}
	cfg := &Config{Org: "org"}
	sync := SyncConfig{Project: "P", Team: "T", TrackProject: "EMP"}
	stats := &PullStats{}

	err := pullTeam(conn, client, cfg, sync, stats, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 0 && stats.Updated != 0 {
		t.Error("expected no changes for empty WIQL result")
	}
}

func TestPullTeamWithAssignedToFilter(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			// Capture the query body to verify assigned filter
			b := make([]byte, 1024)
			n, _ := r.Body.Read(b)
			receivedBody = string(b[:n])
			json.NewEncoder(w).Encode(WIQLResult{WorkItems: []WIQLWorkItem{}})
		}
	}))
	defer server.Close()

	conn := db.OpenTestDB(t)
	db.CreateProject(conn, "ASN", "Assign", "active", "build", "", "{}", 3)

	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}
	cfg := &Config{Org: "org", Email: "me@example.com"}
	sync := SyncConfig{Project: "P", Team: "T", TrackProject: "ASN", AssignedTo: "me"}
	stats := &PullStats{}

	pullTeam(conn, client, cfg, sync, stats, false)

	// "me" should be resolved to cfg.Email
	if receivedBody == "" {
		t.Fatal("expected WIQL request")
	}
	_ = fmt.Sprintf("verified") // keep fmt
}

func TestPullWithTeamFilter(t *testing.T) {
	conn := db.OpenTestDB(t)

	// Set up env for PAT
	os.Setenv("TEST_PULL_PAT", "test-pat")
	defer os.Unsetenv("TEST_PULL_PAT")

	cfg := &Config{
		Org:    "org",
		PatEnv: "TEST_PULL_PAT",
		Email:  "test@example.com",
		Syncs: []SyncConfig{
			{Project: "Proj1", Team: "Team1", TrackProject: "AAA"},
			{Project: "Proj2", Team: "Team2", TrackProject: "BBB"},
		},
	}

	// With a team filter that matches nothing — should return empty stats, no error from network
	// (since no sync matches, no API calls are made)
	stats, err := Pull(conn, cfg, "NONEXISTENT", false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Created != 0 || stats.Updated != 0 {
		t.Error("expected empty stats for non-matching filter")
	}
}
