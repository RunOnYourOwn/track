package ado

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
)

func setupPushTest(t *testing.T, status string, lastSyncedAt string) (*sql.DB, *Config, string) {
	t.Helper()
	conn := db.OpenTestDB(t)

	p, err := db.CreateProject(conn, "PUSH", "Push Test", "active", "build", "", "{}", 3)
	if err != nil {
		t.Fatal(err)
	}

	ctx := AgentContext{
		AdoID:        100,
		AdoRev:       5,
		AdoOrg:       "myorg",
		AdoProject:   "MyProject",
		LastSyncedAt: lastSyncedAt,
	}
	ctxJSON, _ := json.Marshal(ctx)

	_, err = conn.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at)
		VALUES (?, ?, 1, 'Push Task', ?, 'medium', 'task', 'ado', ?, '2026-05-01T00:00:00Z', '2026-05-20T12:00:00Z')`,
		"push-1", p.ID, status, string(ctxJSON))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Org:    "myorg",
		PatEnv: "TRACK_ADO_PAT",
		Email:  "test@example.com",
		Syncs:  []SyncConfig{{Project: "MyProject", Team: "MyTeam", TrackProject: "PUSH"}},
	}

	return conn, cfg, p.ID
}

func TestPushTeamEndToEnd(t *testing.T) {
	// Task is dirty (updated_at well after last_synced_at) and status is in_progress
	conn, cfg, _ := setupPushTest(t, "in_progress", "2026-05-01T00:00:00Z")

	var patchReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json-patch+json" {
			t.Errorf("expected json-patch content type, got %s", r.Header.Get("Content-Type"))
		}

		var ops []PatchOperation
		json.NewDecoder(r.Body).Decode(&ops)
		// Push now sends title + description always, and state when it changed.
		var sawState bool
		for _, op := range ops {
			if op.Path == "/fields/System.State" {
				sawState = true
				if op.Value != "In Progress" {
					t.Errorf("expected state 'In Progress', got %v", op.Value)
				}
			}
		}
		if !sawState {
			t.Errorf("expected a System.State op among: %+v", ops)
		}

		patchReceived = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WorkItem{ID: 100, Rev: 6, Fields: map[string]interface{}{}})
	}))
	defer server.Close()

	t.Setenv("TRACK_ADO_PAT", "testpat")
	client := &Client{org: "myorg", pat: "testpat", httpCli: server.Client(), baseURL: server.URL}

	stats := &PushStats{}
	err := pushTeam(conn, client, cfg, cfg.Syncs[0], stats, false)
	if err != nil {
		t.Fatal(err)
	}

	if !patchReceived {
		t.Error("expected PATCH request")
	}
	if stats.Pushed != 1 {
		t.Errorf("expected 1 pushed, got %d", stats.Pushed)
	}

	// Verify agent_context was updated
	index := db.LoadAdoTaskIndex(conn, "")
	// Re-query directly
	var ctxStr string
	conn.QueryRow(`SELECT agent_context FROM tasks WHERE id = 'push-1'`).Scan(&ctxStr)
	var updatedCtx AgentContext
	json.Unmarshal([]byte(ctxStr), &updatedCtx)
	if updatedCtx.AdoRev != 6 {
		t.Errorf("expected rev 6 after push, got %d", updatedCtx.AdoRev)
	}
	_ = index
}

func TestPushSkipsCleanTasks(t *testing.T) {
	// last_synced_at is AFTER updated_at → not dirty
	conn, cfg, _ := setupPushTest(t, "in_progress", "2026-05-25T00:00:00Z")
	_ = conn

	t.Setenv("TRACK_ADO_PAT", "testpat")
	client := &Client{org: "myorg", pat: "testpat", httpCli: http.DefaultClient, baseURL: "http://unused"}

	stats := &PushStats{}
	err := pushTeam(conn, client, cfg, cfg.Syncs[0], stats, false)
	if err != nil {
		t.Fatal(err)
	}

	if stats.Pushed != 0 {
		t.Errorf("expected 0 pushed, got %d", stats.Pushed)
	}
}

func TestPushUnmappableStatusPushesFieldsOnly(t *testing.T) {
	// Task is dirty but status is 'blocked' (no ADO state mapping): title and
	// description still push; no System.State op is sent.
	conn, cfg, _ := setupPushTest(t, "blocked", "2026-05-01T00:00:00Z")

	var ops []PatchOperation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&ops)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WorkItem{ID: 100, Rev: 6, Fields: map[string]interface{}{}})
	}))
	defer server.Close()

	t.Setenv("TRACK_ADO_PAT", "testpat")
	client := &Client{org: "myorg", pat: "testpat", httpCli: server.Client(), baseURL: server.URL}

	stats := &PushStats{}
	err := pushTeam(conn, client, cfg, cfg.Syncs[0], stats, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, op := range ops {
		if op.Path == "/fields/System.State" {
			t.Errorf("blocked status should not push a state op, got %+v", ops)
		}
	}
	if stats.Pushed != 1 {
		t.Errorf("expected 1 pushed (title/description), got %d", stats.Pushed)
	}
}

func TestPushDryRun(t *testing.T) {
	conn, cfg, _ := setupPushTest(t, "done", "2026-05-01T00:00:00Z")

	var httpCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(500)
	}))
	defer server.Close()

	t.Setenv("TRACK_ADO_PAT", "testpat")
	client := &Client{org: "myorg", pat: "testpat", httpCli: server.Client(), baseURL: server.URL}

	stats := &PushStats{}
	err := pushTeam(conn, client, cfg, cfg.Syncs[0], stats, true)
	if err != nil {
		t.Fatal(err)
	}

	if httpCalled {
		t.Error("expected no HTTP calls in dry-run mode")
	}
	if stats.Pushed != 1 {
		t.Errorf("expected 1 pushed (dry-run counted), got %d", stats.Pushed)
	}

	// Verify agent_context was NOT changed
	var ctxStr string
	conn.QueryRow(`SELECT agent_context FROM tasks WHERE id = 'push-1'`).Scan(&ctxStr)
	var ctx AgentContext
	json.Unmarshal([]byte(ctxStr), &ctx)
	if ctx.AdoRev != 5 {
		t.Errorf("expected rev still 5 in dry-run, got %d", ctx.AdoRev)
	}
}

func TestPushHTTPError(t *testing.T) {
	conn, cfg, _ := setupPushTest(t, "in_progress", "2026-05-01T00:00:00Z")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte("conflict"))
	}))
	defer server.Close()

	t.Setenv("TRACK_ADO_PAT", "testpat")
	client := &Client{org: "myorg", pat: "testpat", httpCli: server.Client(), baseURL: server.URL}

	stats := &PushStats{}
	err := pushTeam(conn, client, cfg, cfg.Syncs[0], stats, false)
	if err != nil {
		t.Fatal(err)
	}

	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.Pushed != 0 {
		t.Errorf("expected 0 pushed, got %d", stats.Pushed)
	}
}

func TestPushNoAdoTasks(t *testing.T) {
	conn := db.OpenTestDB(t)

	_, err := db.CreateProject(conn, "EMPTY", "Empty Project", "active", "build", "", "{}", 3)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Org:    "myorg",
		PatEnv: "TRACK_ADO_PAT",
		Email:  "test@example.com",
		Syncs:  []SyncConfig{{Project: "P", Team: "T", TrackProject: "EMPTY"}},
	}

	t.Setenv("TRACK_ADO_PAT", "testpat")
	client := &Client{org: "myorg", pat: "testpat", httpCli: http.DefaultClient, baseURL: "http://unused"}

	stats := &PushStats{}
	err = pushTeam(conn, client, cfg, cfg.Syncs[0], stats, false)
	if err != nil {
		t.Fatal(err)
	}

	if stats.Pushed != 0 || stats.Skipped != 0 || stats.Failed != 0 {
		t.Errorf("expected all zeros, got pushed=%d skipped=%d failed=%d",
			stats.Pushed, stats.Skipped, stats.Failed)
	}
}

func TestPushFullFlow(t *testing.T) {
	conn := db.OpenTestDB(t)

	p, _ := db.CreateProject(conn, "FLOW", "Flow Test", "active", "build", "", "{}", 3)

	now := time.Now().UTC()
	oldSync := now.Add(-1 * time.Hour).Format(time.RFC3339)

	// Task 1: dirty, in_progress → should push
	ctx1 := AgentContext{AdoID: 10, AdoRev: 2, AdoOrg: "org", AdoProject: "Proj", LastSyncedAt: oldSync}
	ctx1JSON, _ := json.Marshal(ctx1)
	conn.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at)
		VALUES (?, ?, 1, 'T1', 'in_progress', 'medium', 'task', 'ado', ?, ?, ?)`,
		"f-1", p.ID, string(ctx1JSON), oldSync, now.Format(time.RFC3339))

	// Task 2: dirty, todo → no mapping, should skip
	ctx2 := AgentContext{AdoID: 20, AdoRev: 1, AdoOrg: "org", AdoProject: "Proj", LastSyncedAt: oldSync}
	ctx2JSON, _ := json.Marshal(ctx2)
	conn.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at)
		VALUES (?, ?, 2, 'T2', 'todo', 'medium', 'task', 'ado', ?, ?, ?)`,
		"f-2", p.ID, string(ctx2JSON), oldSync, now.Format(time.RFC3339))

	// Task 3: clean (last_synced_at is recent) → should not push
	recentSync := now.Add(1 * time.Hour).Format(time.RFC3339)
	ctx3 := AgentContext{AdoID: 30, AdoRev: 1, AdoOrg: "org", AdoProject: "Proj", LastSyncedAt: recentSync}
	ctx3JSON, _ := json.Marshal(ctx3)
	conn.Exec(`INSERT INTO tasks (id, project_id, seq, title, status, priority, type, source_type, agent_context, created_at, updated_at)
		VALUES (?, ?, 3, 'T3', 'done', 'medium', 'task', 'ado', ?, ?, ?)`,
		"f-3", p.ID, string(ctx3JSON), oldSync, now.Format(time.RFC3339))

	patchCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		patchCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WorkItem{ID: 10, Rev: 3, Fields: map[string]interface{}{}})
	}))
	defer server.Close()

	cfg := &Config{
		Org:    "org",
		PatEnv: "TRACK_ADO_PAT",
		Syncs:  []SyncConfig{{Project: "Proj", Team: "T", TrackProject: "FLOW"}},
	}

	t.Setenv("TRACK_ADO_PAT", "pat")
	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}

	stats := &PushStats{}
	err := pushTeam(conn, client, cfg, cfg.Syncs[0], stats, false)
	if err != nil {
		t.Fatal(err)
	}

	if patchCount != 2 {
		t.Errorf("expected 2 PATCH calls, got %d", patchCount)
	}
	if stats.Pushed != 2 {
		t.Errorf("expected 2 pushed, got %d", stats.Pushed)
	}
	if stats.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", stats.Skipped)
	}
}
