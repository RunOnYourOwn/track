package mcp

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/RunOnYourOwn/track/internal/db"
)

// A panic in a tool handler must come back as a JSON-RPC internal error, not
// crash the stdio session. A tools/call against a nil DB conn panics inside the
// handler (nil *sql.DB query), exercising the recover wrapper.
func TestSafeHandleRequestRecoversPanic(t *testing.T) {
	req := Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"track_project_list","arguments":{}}`),
	}
	resp := safeHandleRequest(nil, req) // nil conn → panic inside the handler
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected a recovered error response, got %+v", resp)
	}
	if resp.Error.Code != -32603 {
		t.Fatalf("expected JSON-RPC internal error -32603, got %d", resp.Error.Code)
	}
}

// Parity: start_date is settable via MCP, and the sprint tools work end-to-end.
func TestSprintToolsAndStartDate(t *testing.T) {
	conn := db.OpenTestDB(t)
	if _, err := db.CreateProject(conn, "SP", "Sprint Proj", "", "", "", "", 3); err != nil {
		t.Fatal(err)
	}

	tres, err := handleTaskCreate(conn, map[string]any{"project": "SP", "title": "T1", "start_date": "2026-06-01"})
	if err != nil {
		t.Fatal(err)
	}
	var task struct {
		ID        string  `json:"id"`
		StartDate *string `json:"start_date"`
	}
	json.Unmarshal([]byte(tres.Content[0].Text), &task)
	if task.StartDate == nil || *task.StartDate != "2026-06-01" {
		t.Fatalf("start_date not set via MCP create: %+v", task)
	}

	sres, err := handleSprintCreate(conn, map[string]any{"project": "SP", "name": "Sprint 1", "goal": "ship"})
	if err != nil {
		t.Fatal(err)
	}
	var sprint struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(sres.Content[0].Text), &sprint)
	if sprint.ID == "" {
		t.Fatalf("sprint create returned no id: %s", sres.Content[0].Text)
	}

	if _, err := handleSprintAdd(conn, map[string]any{"sprint_id": sprint.ID, "task_id": task.ID}); err != nil {
		t.Fatalf("sprint add: %v", err)
	}
	tk, err := handleSprintTasks(conn, map[string]any{"sprint_id": sprint.ID})
	if err != nil {
		t.Fatal(err)
	}
	var sprintTasks []map[string]any
	json.Unmarshal([]byte(tk.Content[0].Text), &sprintTasks)
	if len(sprintTasks) != 1 {
		t.Fatalf("expected 1 task in sprint, got %d", len(sprintTasks))
	}

	st, err := handleSprintStart(conn, map[string]any{"id": sprint.ID})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(st.Content[0].Text), &started)
	if started.Status != "active" {
		t.Fatalf("sprint should be active after start, got %q", started.Status)
	}

	lst, err := handleSprintList(conn, map[string]any{"project": "SP"})
	if err != nil {
		t.Fatal(err)
	}
	var sprints []map[string]any
	json.Unmarshal([]byte(lst.Content[0].Text), &sprints)
	if len(sprints) != 1 {
		t.Fatalf("expected 1 sprint listed, got %d", len(sprints))
	}
}

func mustProjectID(t *testing.T, conn *sql.DB, prefix string) string {
	t.Helper()
	id, err := resolveProjectID(conn, prefix)
	if err != nil {
		t.Fatalf("resolve project %q: %v", prefix, err)
	}
	return id
}

// --- argument coercion helpers ---

func TestArgCoercion(t *testing.T) {
	args := map[string]any{
		"f_float":  float64(3.5),
		"f_int":    7,
		"f_string": "2.5",
		"b_true":   true,
		"s_arr":    []any{"a", "b"},
		"s_csv":    "x,y,z",
		"str":      "hello",
	}
	if got := floatArg(args, "f_float"); got != 3.5 {
		t.Errorf("floatArg float64: got %v", got)
	}
	if got := floatArg(args, "f_int"); got != 7 {
		t.Errorf("floatArg int: got %v", got)
	}
	if got := floatArg(args, "f_string"); got != 2.5 {
		t.Errorf("floatArg string: got %v", got)
	}
	if got := floatArg(args, "missing"); got != 0 {
		t.Errorf("floatArg missing: got %v", got)
	}
	if !boolArg(args, "b_true", false) {
		t.Error("boolArg true")
	}
	if !boolArg(args, "missing", true) {
		t.Error("boolArg should return default for a missing key")
	}
	if got := strSliceArg(args, "s_arr"); len(got) != 2 || got[0] != "a" {
		t.Errorf("strSliceArg array: got %v", got)
	}
	if got := strSliceArg(args, "s_csv"); len(got) != 3 || got[2] != "z" {
		t.Errorf("strSliceArg csv: got %v", got)
	}
	if got := strArg(args, "str"); got != "hello" {
		t.Errorf("strArg: got %v", got)
	}
}

// --- JSON-RPC protocol surface ---

func TestHandleRequestProtocol(t *testing.T) {
	conn := db.OpenTestDB(t)

	resp := handleRequest(conn, Request{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize failed: %+v", resp)
	}

	if r := handleRequest(conn, Request{JSONRPC: "2.0", Method: "notifications/initialized"}); r != nil {
		t.Errorf("notification should yield no response, got %+v", r)
	}

	resp = handleRequest(conn, Request{JSONRPC: "2.0", ID: 2, Method: "tools/list"})
	tl, ok := resp.Result.(ToolsListResult)
	if !ok || len(tl.Tools) == 0 {
		t.Fatalf("tools/list returned no tools: %+v", resp.Result)
	}

	resp = handleRequest(conn, Request{JSONRPC: "2.0", ID: 3, Method: "no/such"})
	if resp == nil || resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601 for unknown method, got %+v", resp)
	}
}

// --- tool handlers + validation ---

func TestTaskCreateValidation(t *testing.T) {
	conn := db.OpenTestDB(t)
	if _, err := db.CreateProject(conn, "MCP", "MCP Proj", "", "", "", "", 3); err != nil {
		t.Fatal(err)
	}

	if _, err := handleTaskCreate(conn, map[string]any{"project": "MCP"}); err == nil {
		t.Error("expected error for missing title")
	}
	if _, err := handleTaskCreate(conn, map[string]any{"title": "x"}); err == nil {
		t.Error("expected error for missing project")
	}
	res, err := handleTaskCreate(conn, map[string]any{"project": "MCP", "title": "Do a thing", "priority": "high"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("expected tool result content")
	}
}

func TestTaskLifecycleViaTools(t *testing.T) {
	conn := db.OpenTestDB(t)
	if _, err := db.CreateProject(conn, "LC", "LC Proj", "", "", "", "", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := handleTaskCreate(conn, map[string]any{"project": "LC", "title": "task one"}); err != nil {
		t.Fatal(err)
	}

	if _, err := handleTaskGet(conn, map[string]any{"id": "LC-1"}); err != nil {
		t.Fatalf("get LC-1: %v", err)
	}
	if _, err := handleTaskMove(conn, map[string]any{"id": "LC-1", "status": "in_progress"}); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := handleTaskMove(conn, map[string]any{"id": "LC-1", "status": "bogus"}); err == nil {
		t.Error("expected invalid status to be rejected")
	}
	if _, err := handleTaskDone(conn, map[string]any{"id": "LC-1"}); err != nil {
		t.Fatalf("done: %v", err)
	}
}

// H10 regression: ending a session with a wrong/typo'd project must error,
// not silently end an unrelated project's active session.
func TestSessionEndWrongProjectErrors(t *testing.T) {
	conn := db.OpenTestDB(t)
	if _, err := db.CreateProject(conn, "AAA", "A", "", "", "", "", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := handleSessionStart(conn, map[string]any{"project": "AAA"}); err != nil {
		t.Fatal(err)
	}

	if _, err := handleSessionEnd(conn, map[string]any{"project": "NOPE"}); err == nil {
		t.Fatal("session_end with unknown project should error")
	}

	sess, _ := db.GetCurrentSession(conn, mustProjectID(t, conn, "AAA"))
	if sess == nil {
		t.Fatal("AAA session should still be active after a failed session_end")
	}
}

func TestResolveTaskID(t *testing.T) {
	conn := db.OpenTestDB(t)
	if _, err := db.CreateProject(conn, "RID", "R", "", "", "", "", 3); err != nil {
		t.Fatal(err)
	}
	tk, err := db.CreateTask(conn, db.CreateTaskOpts{ProjectID: mustProjectID(t, conn, "RID"), Title: "t"})
	if err != nil {
		t.Fatal(err)
	}

	if got, err := resolveTaskID(conn, tk.ID); err != nil || got != tk.ID {
		t.Errorf("ULID passthrough: got %q err %v", got, err)
	}
	if got, err := resolveTaskID(conn, "RID-1"); err != nil || got != tk.ID {
		t.Errorf("display id resolve: got %q err %v", got, err)
	}
	if _, err := resolveTaskID(conn, "not-a-task"); err == nil {
		t.Error("expected error for invalid id")
	}
}
