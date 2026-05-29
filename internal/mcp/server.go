package mcp

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/RunOnYourOwn/track/internal/version"
)

// JSON-RPC 2.0 types

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]PropertySchema `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

type PropertySchema struct {
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Items       *ItemSchema `json:"items,omitempty"`
}

type ItemSchema struct {
	Type string `json:"type"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Run starts the MCP stdio server loop. It blocks until stdin is closed.
func Run() error {
	conn, err := db.Open()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 10*1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(Response{
				JSONRPC: "2.0",
				Error:   &Error{Code: -32700, Message: "parse error: " + err.Error()},
			})
			continue
		}

		resp := safeHandleRequest(conn, req)
		// Notifications have no ID and no response needed (ID is nil and method starts with "notifications/")
		if resp == nil {
			continue
		}
		_ = enc.Encode(resp)
	}
	// Surface why the loop ended (e.g. bufio.ErrTooLong on a >10MB line) on
	// stderr so the operator isn't left guessing why the server went quiet.
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: input scan stopped: %v\n", err)
		return err
	}
	return nil
}

// safeHandleRequest runs handleRequest with panic recovery, so a panic in any
// single tool handler returns a JSON-RPC internal error instead of crashing the
// whole stdio session (which would silently kill the connected agent).
func safeHandleRequest(conn *sql.DB, req Request) (resp *Response) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "mcp: recovered from panic handling %q: %v\n", req.Method, r)
			if req.ID == nil { // notification — no response expected
				resp = nil
				return
			}
			resp = &Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &Error{Code: -32603, Message: fmt.Sprintf("internal error: %v", r)},
			}
		}
	}()
	return handleRequest(conn, req)
}

func handleRequest(conn *sql.DB, req Request) *Response {
	base := Response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		base.Result = InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: "track", Version: version.String()},
			Capabilities:    Capabilities{Tools: &struct{}{}},
		}
		return &base

	case "notifications/initialized":
		// Client confirmation — no response needed
		return nil

	case "tools/list":
		base.Result = ToolsListResult{Tools: allTools()}
		return &base

	case "tools/call":
		result, toolErr := dispatchTool(conn, req.Params)
		if toolErr != nil {
			base.Result = ToolCallResult{
				Content: []ContentItem{{Type: "text", Text: toolErr.Error()}},
				IsError: true,
			}
		} else {
			base.Result = result
		}
		return &base

	default:
		base.Error = &Error{Code: -32601, Message: "method not found: " + req.Method}
		return &base
	}
}

// dispatchTool routes a tools/call request to the right handler.
func dispatchTool(conn *sql.DB, raw json.RawMessage) (*ToolCallResult, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}

	// Parse arguments into a generic map
	var args map[string]any
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	switch call.Name {
	case "track_project_list":
		return handleProjectList(conn, args)
	case "track_project_create":
		return handleProjectCreate(conn, args)
	case "track_project_delete":
		return handleProjectDelete(conn, args)
	case "track_project_update":
		return handleProjectUpdate(conn, args)
	case "track_task_list":
		return handleTaskList(conn, args)
	case "track_task_create":
		return handleTaskCreate(conn, args)
	case "track_task_get":
		return handleTaskGet(conn, args)
	case "track_task_move":
		return handleTaskMove(conn, args)
	case "track_task_done":
		return handleTaskDone(conn, args)
	case "track_task_cancel":
		return handleTaskCancel(conn, args)
	case "track_task_next":
		return handleTaskNext(conn, args)
	case "track_task_link":
		return handleTaskLink(conn, args)
	case "track_task_unlink":
		return handleTaskUnlink(conn, args)
	case "track_task_delete":
		return handleTaskDelete(conn, args)
	case "track_task_update":
		return handleTaskUpdate(conn, args)
	case "track_session_start":
		return handleSessionStart(conn, args)
	case "track_session_end":
		return handleSessionEnd(conn, args)
	case "track_session_log":
		return handleSessionLog(conn, args)
	case "track_session_current":
		return handleSessionCurrent(conn, args)
	case "track_decision_create":
		return handleDecisionCreate(conn, args)
	case "track_decision_resolve":
		return handleDecisionResolve(conn, args)
	case "track_learn":
		return handleLearn(conn, args)
	case "track_learn_search":
		return handleLearnSearch(conn, args)
	case "track_status":
		return handleStatus(conn, args)
	case "track_blocker_list":
		return handleBlockerList(conn, args)
	case "track_blocker_create":
		return handleBlockerCreate(conn, args)
	case "track_blocker_resolve":
		return handleBlockerResolve(conn, args)
	case "track_decision_list":
		return handleDecisionList(conn, args)
	case "track_decision_update":
		return handleDecisionUpdate(conn, args)
	case "track_learn_list":
		return handleLearnList(conn, args)
	case "track_learn_update":
		return handleLearnUpdate(conn, args)
	case "track_sprint_create":
		return handleSprintCreate(conn, args)
	case "track_sprint_list":
		return handleSprintList(conn, args)
	case "track_sprint_start":
		return handleSprintStart(conn, args)
	case "track_sprint_complete":
		return handleSprintComplete(conn, args)
	case "track_sprint_add":
		return handleSprintAdd(conn, args)
	case "track_sprint_remove":
		return handleSprintRemove(conn, args)
	case "track_sprint_tasks":
		return handleSprintTasks(conn, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

// --- helpers ---

func strArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func floatArg(args map[string]any, key string) float64 {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch f := v.(type) {
	case float64:
		return f
	case int:
		return float64(f)
	case string:
		n, _ := strconv.ParseFloat(f, 64)
		return n
	}
	return 0
}

func boolArg(args map[string]any, key string, def bool) bool {
	v, ok := args[key]
	if !ok {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}

func strSliceArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		// Try comma-separated string
		s, ok := v.(string)
		if !ok || s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(b)
}

func textResult(text string) *ToolCallResult {
	return &ToolCallResult{Content: []ContentItem{{Type: "text", Text: text}}}
}

func jsonResult(v any) *ToolCallResult {
	return textResult(toJSON(v))
}

// resolveTaskID resolves PREFIX-NNN or raw ULID to an internal task ID.
func resolveTaskID(conn *sql.DB, displayID string) (string, error) {
	if len(displayID) == 26 && !strings.Contains(displayID, "-") {
		var exists int
		err := conn.QueryRow(`SELECT 1 FROM tasks WHERE id = ?`, displayID).Scan(&exists)
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("task %q not found", displayID)
		}
		if err != nil {
			return "", fmt.Errorf("task lookup: %w", err)
		}
		return displayID, nil
	}
	parts := strings.SplitN(displayID, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid task ID %q (expected PREFIX-NNN or ULID)", displayID)
	}
	seq, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid seq in %q", displayID)
	}
	task, err := db.GetTaskByDisplayID(conn, parts[0], seq)
	if err != nil {
		return "", fmt.Errorf("task %q not found", displayID)
	}
	return task.ID, nil
}

// resolveProjectID maps a prefix string to a project ID.
func resolveProjectID(conn *sql.DB, prefix string) (string, error) {
	if prefix == "" {
		// If only one project exists, use it
		projects, err := db.ListProjects(conn)
		if err != nil {
			return "", err
		}
		if len(projects) == 1 {
			return projects[0].ID, nil
		}
		return "", fmt.Errorf("project prefix required (multiple projects exist)")
	}
	p, err := db.GetProjectByPrefix(conn, prefix)
	if err != nil {
		return "", fmt.Errorf("project %q not found", prefix)
	}
	return p.ID, nil
}

// --- tool handlers ---

func handleProjectList(conn *sql.DB, _ map[string]any) (*ToolCallResult, error) {
	projects, err := db.ListProjects(conn)
	if err != nil {
		return nil, err
	}
	return jsonResult(projects), nil
}

// handleProjectCreate mirrors the CLI `project create`: prefix + name are
// required; phase/phase_type are optional; wip_limit defaults inside
// db.CreateProject (0 → 3) just as the CLI's default flag value does.
func handleProjectCreate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "prefix")
	if prefix == "" {
		return nil, fmt.Errorf("prefix is required")
	}
	name := strArg(args, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	phaseType := strArg(args, "phase_type")
	if phaseType != "" && !db.ValidPhaseTypes[phaseType] {
		return nil, fmt.Errorf("invalid phase_type %q (expected: discovery, design, build, stabilize, maintain)", phaseType)
	}
	wipLimit := int(floatArg(args, "wip_limit"))
	p, err := db.CreateProject(conn, prefix, name, strArg(args, "phase"), phaseType, "", "", wipLimit)
	if err != nil {
		return nil, err
	}
	return jsonResult(p), nil
}

// handleProjectDelete cascades a full project deletion. Because an MCP tool can't
// prompt the user, it requires the caller to pass `confirm` equal to the prefix —
// a deliberate echo that guards against an accidental call. The agent must still
// get the user's go-ahead before invoking this; the check just prevents fat-finger
// deletions, it is not user consent.
// handleProjectUpdate edits project settings (only the args provided change),
// mirroring the CLI `project edit` and the HTTP PATCH so MCP agents have parity.
func handleProjectUpdate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "prefix")
	if prefix == "" {
		return nil, fmt.Errorf("prefix is required")
	}
	p, err := db.GetProjectByPrefix(conn, prefix)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", prefix)
	}
	if v := strArg(args, "name"); v != "" {
		if err := db.UpdateProjectField(conn, p.ID, "name", v); err != nil {
			return nil, err
		}
	}
	if v := strArg(args, "phase"); v != "" {
		if err := db.UpdateProjectField(conn, p.ID, "phase", v); err != nil {
			return nil, err
		}
	}
	if v := strArg(args, "phase_type"); v != "" {
		if !db.ValidPhaseTypes[v] {
			return nil, fmt.Errorf("invalid phase_type %q (expected: discovery, design, build, stabilize, maintain)", v)
		}
		if err := db.UpdateProjectField(conn, p.ID, "phase_type", v); err != nil {
			return nil, err
		}
	}
	if v := strArg(args, "task_sort"); v != "" {
		if !db.ValidTaskSorts[v] {
			return nil, fmt.Errorf("invalid task_sort %q (expected: priority, manual, created, due)", v)
		}
		if err := db.UpdateProjectField(conn, p.ID, "task_sort", v); err != nil {
			return nil, err
		}
	}
	if wip := floatArg(args, "wip_limit"); wip >= 1 {
		if err := db.UpdateProjectField(conn, p.ID, "wip_limit", fmt.Sprintf("%d", int(wip))); err != nil {
			return nil, err
		}
	}
	updated, err := db.GetProjectByID(conn, p.ID)
	if err != nil {
		return nil, err
	}
	return jsonResult(updated), nil
}

func handleProjectDelete(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "prefix")
	if prefix == "" {
		return nil, fmt.Errorf("prefix is required")
	}
	if strArg(args, "confirm") != prefix {
		return nil, fmt.Errorf("confirmation required: pass confirm equal to the prefix %q to delete it and ALL its data", prefix)
	}
	p, err := db.GetProjectByPrefix(conn, prefix)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", prefix)
	}
	if err := db.DeleteProject(conn, p.ID); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("deleted project %s and all its data", prefix)), nil
}

func handleTaskList(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	opts := db.ListTaskOpts{}

	prefix := strArg(args, "project")
	if prefix != "" {
		projectID, err := resolveProjectID(conn, prefix)
		if err != nil {
			return nil, err
		}
		opts.ProjectID = projectID
	}

	if status := strArg(args, "status"); status != "" {
		opts.Status = strings.Split(status, ",")
	}
	if priority := strArg(args, "priority"); priority != "" {
		opts.Priority = strings.Split(priority, ",")
	}

	tasks, err := db.ListTasks(conn, opts)
	if err != nil {
		return nil, err
	}
	return jsonResult(tasks), nil
}

func handleTaskCreate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	if prefix == "" {
		return nil, fmt.Errorf("project is required")
	}
	title := strArg(args, "title")
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}

	opts := db.CreateTaskOpts{
		ProjectID:            projectID,
		Title:                title,
		Description:          strArg(args, "description"),
		Priority:             strArg(args, "priority"),
		Type:                 strArg(args, "type"),
		EstimateSize:         strArg(args, "estimate"),
		EstimateHours:        floatArg(args, "hours"),
		EstimateAgentMinutes: int(floatArg(args, "estimate_agent_minutes")),
		SourceType:           strArg(args, "source"),
		AgentContext:         strArg(args, "agent_context"),
		StartDate:            strArg(args, "start_date"),
		DueDate:              strArg(args, "due_date"),
	}

	if parentStr := strArg(args, "parent_id"); parentStr != "" {
		parentID, err := resolveTaskID(conn, parentStr)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		opts.ParentID = parentID
	}

	task, err := db.CreateTask(conn, opts)
	if err != nil {
		return nil, err
	}
	return jsonResult(task), nil
}

func handleTaskGet(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	taskID, err := resolveTaskID(conn, idStr)
	if err != nil {
		return nil, err
	}
	task, err := db.GetTask(conn, taskID)
	if err != nil {
		return nil, err
	}
	return jsonResult(task), nil
}

func handleTaskMove(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	status := strArg(args, "status")
	if status == "" {
		return nil, fmt.Errorf("status is required")
	}

	taskID, err := resolveTaskID(conn, idStr)
	if err != nil {
		return nil, err
	}
	if err := db.MoveTask(conn, taskID, status); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("moved %s to %s", idStr, status)), nil
}

func handleTaskDone(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	taskID, err := resolveTaskID(conn, idStr)
	if err != nil {
		return nil, err
	}
	actualHours := floatArg(args, "actual_hours")
	if err := db.CompleteTask(conn, taskID, actualHours, strArg(args, "note")); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("completed %s", idStr)), nil
}

func handleTaskCancel(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	taskID, err := resolveTaskID(conn, idStr)
	if err != nil {
		return nil, err
	}
	if err := db.CancelTask(conn, taskID, strArg(args, "reason")); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("cancelled %s", idStr)), nil
}

func handleTaskNext(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	var projectID string

	if prefix != "" {
		var err error
		projectID, err = resolveProjectID(conn, prefix)
		if err != nil {
			return nil, err
		}
	} else {
		projects, err := db.ListProjects(conn)
		if err != nil {
			return nil, err
		}
		switch len(projects) {
		case 0:
			return nil, fmt.Errorf("no projects — create one first")
		case 1:
			projectID = projects[0].ID
		default:
			return nil, fmt.Errorf("multiple projects — specify project")
		}
	}

	task, err := db.SuggestNext(conn, projectID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return textResult("no available tasks (all done or blocked)"), nil
	}
	return jsonResult(task), nil
}

func handleTaskLink(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	fromStr := strArg(args, "from_id")
	toStr := strArg(args, "to_id")
	if fromStr == "" || toStr == "" {
		return nil, fmt.Errorf("from_id and to_id are required")
	}

	fromID, err := resolveTaskID(conn, fromStr)
	if err != nil {
		return nil, fmt.Errorf("from_id: %w", err)
	}
	toID, err := resolveTaskID(conn, toStr)
	if err != nil {
		return nil, fmt.Errorf("to_id: %w", err)
	}

	depType := strArg(args, "type")
	reason := strArg(args, "reason")

	if err := db.CreateDependency(conn, fromID, toID, depType, reason); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("linked: %s blocks %s", fromStr, toStr)), nil
}

func handleTaskUnlink(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	fromStr := strArg(args, "from_id")
	toStr := strArg(args, "to_id")
	if fromStr == "" || toStr == "" {
		return nil, fmt.Errorf("from_id and to_id are required")
	}

	fromID, err := resolveTaskID(conn, fromStr)
	if err != nil {
		return nil, fmt.Errorf("from_id: %w", err)
	}
	toID, err := resolveTaskID(conn, toStr)
	if err != nil {
		return nil, fmt.Errorf("to_id: %w", err)
	}

	if err := db.DeleteDependency(conn, fromID, toID); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("unlinked: %s no longer blocks %s", fromStr, toStr)), nil
}

func handleTaskDelete(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	taskID, err := resolveTaskID(conn, idStr)
	if err != nil {
		return nil, err
	}
	if err := db.DeleteTask(conn, taskID); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("deleted task %s", idStr)), nil
}

func handleTaskUpdate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	taskID, err := resolveTaskID(conn, idStr)
	if err != nil {
		return nil, err
	}

	updated := 0
	setField := func(field, value string) error {
		if err := db.UpdateTaskField(conn, taskID, field, value); err != nil {
			return fmt.Errorf("updating %s: %w", field, err)
		}
		updated++
		return nil
	}

	// Enum / required fields: apply only when a non-empty value is given — an empty
	// value is invalid (nothing to clear to). Mirrors the HTTP plain-string set.
	for _, f := range []string{"title", "type", "priority", "tags"} {
		if v := strArg(args, f); v != "" {
			if err := setField(f, v); err != nil {
				return nil, err
			}
		}
	}

	// Clearable fields: apply whenever the caller provides the key (even ""/0) so a
	// value can be cleared/zeroed — presence detection mirrors the HTTP pointer set.
	for _, f := range []string{"description", "estimate_size", "start_date", "due_date"} {
		if _, ok := args[f]; ok {
			if err := setField(f, strArg(args, f)); err != nil {
				return nil, err
			}
		}
	}
	if _, ok := args["estimate_hours"]; ok {
		if err := setField("estimate_hours", strconv.FormatFloat(floatArg(args, "estimate_hours"), 'f', -1, 64)); err != nil {
			return nil, err
		}
	}
	if _, ok := args["estimate_agent_minutes"]; ok {
		if err := setField("estimate_agent_minutes", strconv.Itoa(int(floatArg(args, "estimate_agent_minutes")))); err != nil {
			return nil, err
		}
	}
	if _, ok := args["sort_order"]; ok {
		if err := setField("sort_order", strconv.Itoa(int(floatArg(args, "sort_order")))); err != nil {
			return nil, err
		}
	}

	if parentStr := strArg(args, "parent_id"); parentStr != "" {
		parentID := ""
		if parentStr != "null" && parentStr != "" {
			parentID, err = resolveTaskID(conn, parentStr)
			if err != nil {
				return nil, fmt.Errorf("parent_id: %w", err)
			}
		}
		if err := db.SetParentID(conn, taskID, parentID); err != nil {
			return nil, err
		}
		updated++
	}

	task, err := db.GetTask(conn, taskID)
	if err != nil {
		return nil, err
	}
	return jsonResult(task), nil
}

func handleSessionStart(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	if prefix == "" {
		return nil, fmt.Errorf("project is required")
	}
	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}

	existing, _ := db.GetCurrentSession(conn, projectID)
	if existing != nil {
		return nil, fmt.Errorf("session already active (started %s) — end it first", existing.StartedAt.Format("2006-01-02 15:04"))
	}

	branch := strArg(args, "branch")
	session, err := db.StartSession(conn, projectID, branch)
	if err != nil {
		return nil, err
	}
	return jsonResult(session), nil
}

func handleSessionEnd(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	project := strArg(args, "project")
	// Only resolve when a project was given. An empty project intentionally means
	// "end the most recent active session across all projects"; a NON-empty but
	// unresolvable prefix must error rather than silently fall back to "" (which
	// would end an unrelated project's session).
	var projectID string
	if project != "" {
		pid, err := resolveProjectID(conn, project)
		if err != nil {
			return nil, err
		}
		projectID = pid
	}
	session, err := db.GetCurrentSession(conn, projectID)
	if err != nil || session == nil {
		return nil, fmt.Errorf("no active session")
	}
	summary := strArg(args, "summary")
	if err := db.EndSession(conn, session.ID, summary); err != nil {
		return nil, err
	}
	return textResult("session ended"), nil
}

func handleSessionLog(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	taskIDStr := strArg(args, "task_id")
	if taskIDStr == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	taskID, err := resolveTaskID(conn, taskIDStr)
	if err != nil {
		return nil, err
	}

	hours := floatArg(args, "hours")
	note := strArg(args, "note")

	// Attribute to the active session of the task's OWN project, not just the
	// globally most-recent session (which could belong to a different project).
	var sessionID string
	if task, err := db.GetTask(conn, taskID); err == nil {
		if sess, _ := db.GetCurrentSession(conn, task.ProjectID); sess != nil {
			sessionID = sess.ID
		}
	}

	if err := db.LogTime(conn, taskID, sessionID, hours, note); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("logged %.2fh to %s", hours, taskIDStr)), nil
}

func handleSessionCurrent(conn *sql.DB, _ map[string]any) (*ToolCallResult, error) {
	session, err := db.GetCurrentSession(conn, "")
	if err != nil {
		return nil, err
	}
	if session == nil {
		return textResult("no active session"), nil
	}
	return jsonResult(session), nil
}

func handleDecisionCreate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	if prefix == "" {
		return nil, fmt.Errorf("project is required")
	}
	title := strArg(args, "title")
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}

	decision, err := db.CreateDecision(conn, db.CreateDecisionOpts{
		ProjectID: projectID,
		Title:     title,
		Context:   strArg(args, "context"),
		RevisitBy: strArg(args, "revisit_by"),
	})
	if err != nil {
		return nil, err
	}
	return jsonResult(decision), nil
}

func handleDecisionResolve(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	id := strArg(args, "id")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	decision := strArg(args, "decision")
	if decision == "" {
		return nil, fmt.Errorf("decision is required")
	}
	rationale := strArg(args, "rationale")
	if rationale == "" {
		return nil, fmt.Errorf("rationale is required")
	}
	if err := db.ResolveDecision(conn, id, decision, rationale); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("decision %s resolved", id)), nil
}

func handleLearn(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	if prefix == "" {
		return nil, fmt.Errorf("project is required")
	}
	title := strArg(args, "title")
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	body := strArg(args, "body")
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}

	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}

	// applies_to: accept array or comma-string
	var appliesTo string
	if arr := strSliceArg(args, "applies_to"); len(arr) > 0 {
		b, _ := json.Marshal(arr)
		appliesTo = string(b)
	}

	learning, err := db.CreateLearning(conn, db.CreateLearningOpts{
		ProjectID: projectID,
		Title:     title,
		Body:      body,
		Category:  strArg(args, "category"),
		AppliesTo: appliesTo,
	})
	if err != nil {
		return nil, err
	}
	return jsonResult(learning), nil
}

func handleLearnSearch(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	query := strArg(args, "query")
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	learnings, err := db.SearchLearnings(conn, "", query)
	if err != nil {
		return nil, err
	}
	return jsonResult(learnings), nil
}

func handleStatus(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")

	projects, err := db.ListProjects(conn)
	if err != nil {
		return nil, err
	}

	type projectStatus struct {
		Prefix string         `json:"prefix"`
		Name   string         `json:"name"`
		Phase  string         `json:"phase"`
		Tasks  map[string]int `json:"tasks"`
		Total  int            `json:"total"`
	}

	var results []projectStatus
	for _, p := range projects {
		if prefix != "" && !strings.EqualFold(p.Prefix, prefix) {
			continue
		}

		tasks, err := db.ListTasks(conn, db.ListTaskOpts{ProjectID: p.ID})
		if err != nil {
			return nil, fmt.Errorf("list tasks for %s: %w", p.Prefix, err)
		}

		counts := map[string]int{
			"todo":        0,
			"in_progress": 0,
			"blocked":     0,
			"waiting":     0,
			"done":        0,
		}
		for _, t := range tasks {
			if strings.HasPrefix(t.Status, "waiting") {
				counts["waiting"]++
			} else {
				counts[t.Status]++
			}
		}

		results = append(results, projectStatus{
			Prefix: p.Prefix,
			Name:   p.Name,
			Phase:  p.Phase,
			Tasks:  counts,
			Total:  len(tasks),
		})
	}

	return jsonResult(results), nil
}

func handleBlockerList(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	openOnly := boolArg(args, "open", true)

	if prefix == "" {
		// List blockers for all projects
		projects, err := db.ListProjects(conn)
		if err != nil {
			return nil, err
		}
		type projectBlockers struct {
			Project  string        `json:"project"`
			Blockers []interface{} `json:"blockers"`
		}
		var allBlockers []interface{}
		for _, p := range projects {
			blockers, err := db.ListBlockers(conn, p.ID, openOnly)
			if err != nil {
				return nil, fmt.Errorf("list blockers for %s: %w", p.Prefix, err)
			}
			for _, b := range blockers {
				allBlockers = append(allBlockers, b)
			}
		}
		return jsonResult(allBlockers), nil
	}

	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}

	blockers, err := db.ListBlockers(conn, projectID, openOnly)
	if err != nil {
		return nil, err
	}
	return jsonResult(blockers), nil
}

func handleBlockerCreate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	if prefix == "" {
		return nil, fmt.Errorf("project is required")
	}
	title := strArg(args, "title")
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}
	taskID := ""
	if t := strArg(args, "task"); t != "" {
		if taskID, err = resolveTaskID(conn, t); err != nil {
			return nil, err
		}
	}
	blockerType := strArg(args, "type")
	if blockerType == "" {
		blockerType = "external"
	}
	b, err := db.CreateBlocker(conn, projectID, title, blockerType, taskID,
		strArg(args, "owner"), strArg(args, "escalation_date"), strArg(args, "notes"))
	if err != nil {
		return nil, err
	}
	return jsonResult(b), nil
}

func handleBlockerResolve(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	id := strArg(args, "id")
	if id == "" {
		return nil, fmt.Errorf("id is required (full blocker id from track_blocker_list)")
	}
	if err := db.ResolveBlocker(conn, id); err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("resolved blocker %s", id)), nil
}

func handleDecisionList(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	var statuses []string
	if s := strArg(args, "status"); s != "" {
		statuses = strings.Split(s, ",")
	}
	expiring := boolArg(args, "expiring", false)

	if prefix := strArg(args, "project"); prefix != "" {
		projectID, err := resolveProjectID(conn, prefix)
		if err != nil {
			return nil, err
		}
		ds, err := db.ListDecisions(conn, projectID, statuses, expiring)
		if err != nil {
			return nil, err
		}
		return jsonResult(ds), nil
	}

	projects, err := db.ListProjects(conn)
	if err != nil {
		return nil, err
	}
	var all []interface{}
	for _, p := range projects {
		ds, err := db.ListDecisions(conn, p.ID, statuses, expiring)
		if err != nil {
			return nil, fmt.Errorf("list decisions for %s: %w", p.Prefix, err)
		}
		for _, d := range ds {
			all = append(all, d)
		}
	}
	return jsonResult(all), nil
}

func handleLearnList(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	category := strArg(args, "category")

	if prefix := strArg(args, "project"); prefix != "" {
		projectID, err := resolveProjectID(conn, prefix)
		if err != nil {
			return nil, err
		}
		ls, err := db.ListLearnings(conn, projectID, category)
		if err != nil {
			return nil, err
		}
		return jsonResult(ls), nil
	}

	projects, err := db.ListProjects(conn)
	if err != nil {
		return nil, err
	}
	var all []interface{}
	for _, p := range projects {
		ls, err := db.ListLearnings(conn, p.ID, category)
		if err != nil {
			return nil, fmt.Errorf("list learnings for %s: %w", p.Prefix, err)
		}
		for _, l := range ls {
			all = append(all, l)
		}
	}
	return jsonResult(all), nil
}

func handleDecisionUpdate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	id := strArg(args, "id")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if _, err := db.GetDecision(conn, id); err != nil {
		return nil, fmt.Errorf("decision %q not found", id)
	}
	for _, field := range []string{"title", "context", "options", "revisit_by", "decided_by"} {
		if v := strArg(args, field); v != "" {
			if err := db.UpdateDecisionField(conn, id, field, v); err != nil {
				return nil, err
			}
		}
	}
	d, err := db.GetDecision(conn, id)
	if err != nil {
		return nil, err
	}
	return jsonResult(d), nil
}

func handleLearnUpdate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	id := strArg(args, "id")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if _, err := db.GetLearning(conn, id); err != nil {
		return nil, fmt.Errorf("learning %q not found", id)
	}
	for _, field := range []string{"title", "body", "category", "applies_to"} {
		if v := strArg(args, field); v != "" {
			if err := db.UpdateLearningField(conn, id, field, v); err != nil {
				return nil, err
			}
		}
	}
	l, err := db.GetLearning(conn, id)
	if err != nil {
		return nil, err
	}
	return jsonResult(l), nil
}

// --- sprint tools (parity with the CLI sprint subcommands) ---

func handleSprintCreate(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	name := strArg(args, "name")
	if prefix == "" || name == "" {
		return nil, fmt.Errorf("project and name are required")
	}
	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}
	s, err := db.CreateSprint(conn, db.CreateSprintOpts{
		ProjectID: projectID,
		Name:      name,
		Goal:      strArg(args, "goal"),
		StartDate: strArg(args, "start_date"),
		EndDate:   strArg(args, "end_date"),
	})
	if err != nil {
		return nil, err
	}
	return jsonResult(s), nil
}

func handleSprintList(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	prefix := strArg(args, "project")
	if prefix == "" {
		return nil, fmt.Errorf("project is required")
	}
	projectID, err := resolveProjectID(conn, prefix)
	if err != nil {
		return nil, err
	}
	sprints, err := db.ListSprints(conn, projectID)
	if err != nil {
		return nil, err
	}
	return jsonResult(sprints), nil
}

func handleSprintStart(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	return sprintSetStatus(conn, args, "active")
}

func handleSprintComplete(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	return sprintSetStatus(conn, args, "completed")
}

func sprintSetStatus(conn *sql.DB, args map[string]any, status string) (*ToolCallResult, error) {
	idStr := strArg(args, "id")
	if idStr == "" {
		return nil, fmt.Errorf("id is required")
	}
	sid, err := db.ResolveSprintID(conn, idStr)
	if err != nil {
		return nil, err
	}
	if err := db.UpdateSprintStatus(conn, sid, status); err != nil {
		return nil, err
	}
	s, err := db.GetSprint(conn, sid)
	if err != nil {
		return nil, err
	}
	return jsonResult(s), nil
}

func handleSprintAdd(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	return sprintTaskMembership(conn, args, true)
}

func handleSprintRemove(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	return sprintTaskMembership(conn, args, false)
}

func sprintTaskMembership(conn *sql.DB, args map[string]any, add bool) (*ToolCallResult, error) {
	sprintStr := strArg(args, "sprint_id")
	taskStr := strArg(args, "task_id")
	if sprintStr == "" || taskStr == "" {
		return nil, fmt.Errorf("sprint_id and task_id are required")
	}
	sid, err := db.ResolveSprintID(conn, sprintStr)
	if err != nil {
		return nil, err
	}
	taskID, err := resolveTaskID(conn, taskStr)
	if err != nil {
		return nil, err
	}
	verb := "removed"
	if add {
		err = db.AddTaskToSprint(conn, sid, taskID)
		verb = "added"
	} else {
		err = db.RemoveTaskFromSprint(conn, sid, taskID)
	}
	if err != nil {
		return nil, err
	}
	return textResult(fmt.Sprintf("Task %s %s sprint %s.", taskStr, verb, sprintStr)), nil
}

func handleSprintTasks(conn *sql.DB, args map[string]any) (*ToolCallResult, error) {
	sprintStr := strArg(args, "sprint_id")
	if sprintStr == "" {
		return nil, fmt.Errorf("sprint_id is required")
	}
	sid, err := db.ResolveSprintID(conn, sprintStr)
	if err != nil {
		return nil, err
	}
	tasks, err := db.ListSprintTasks(conn, sid)
	if err != nil {
		return nil, err
	}
	return jsonResult(tasks), nil
}

// --- tool definitions ---

func allTools() []Tool {
	return []Tool{
		{
			Name:        "track_project_list",
			Description: "List all projects",
			InputSchema: InputSchema{Type: "object"},
		},
		{
			Name:        "track_project_create",
			Description: "Create a new project. prefix + name are required; wip_limit defaults to 3.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"prefix":     {Type: "string", Description: "Project prefix, e.g. PROJ (required)"},
					"name":       {Type: "string", Description: "Project name (required)"},
					"phase":      {Type: "string", Description: "Current phase label (e.g. MVP1)"},
					"phase_type": {Type: "string", Description: "discovery | design | build | stabilize | maintain (default: build)"},
					"wip_limit":  {Type: "number", Description: "Max in-progress tasks (default: 3)"},
				},
				Required: []string{"prefix", "name"},
			},
		},
		{
			Name:        "track_project_update",
			Description: "Edit project settings; only the fields you pass change.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"prefix":     {Type: "string", Description: "Project prefix (required)"},
					"name":       {Type: "string", Description: "Project name"},
					"phase":      {Type: "string", Description: "Current phase label (e.g. MVP1)"},
					"phase_type": {Type: "string", Description: "discovery | design | build | stabilize | maintain"},
					"wip_limit":  {Type: "number", Description: "Max in-progress tasks (>= 1)"},
					"task_sort":  {Type: "string", Description: "priority | manual | created | due"},
				},
				Required: []string{"prefix"},
			},
		},
		{
			Name:        "track_project_delete",
			Description: "Permanently delete a project and ALL its data (every task, sprint, session, decision, learning, blocker). Irreversible. Confirm with the user first, then pass confirm equal to the prefix.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"prefix":  {Type: "string", Description: "Project prefix to delete"},
					"confirm": {Type: "string", Description: "Must equal prefix to proceed (guards against accidental deletion)"},
				},
				Required: []string{"prefix", "confirm"},
			},
		},
		{
			Name:        "track_task_list",
			Description: "List tasks, optionally filtered by project, status, or priority",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":  {Type: "string", Description: "Project prefix (e.g. PROJ)"},
					"status":   {Type: "string", Description: "Comma-separated statuses: todo,in_progress,blocked,waiting_review,waiting_external,waiting_dependency,done,cancelled"},
					"priority": {Type: "string", Description: "Comma-separated priorities: urgent,high,medium,low"},
				},
			},
		},
		{
			Name:        "track_task_create",
			Description: "Create a new task",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":                {Type: "string", Description: "Project prefix (required)"},
					"title":                  {Type: "string", Description: "Task title (required)"},
					"type":                   {Type: "string", Description: "epic | feature | task (default: task)"},
					"priority":               {Type: "string", Description: "urgent | high | medium | low"},
					"estimate":               {Type: "string", Description: "T-shirt size: XS | S | M | L | XL"},
					"hours":                  {Type: "number", Description: "Estimated hours (human)"},
					"estimate_agent_minutes": {Type: "number", Description: "Estimated agent minutes (drives accuracy + rollup)"},
					"description":            {Type: "string", Description: "Task description"},
					"source":                 {Type: "string", Description: "planned | discovered | stakeholder | bug | debt"},
					"agent_context":          {Type: "string", Description: "Agent context JSON"},
					"parent_id":              {Type: "string", Description: "Parent task ID (PREFIX-NNN or ULID)"},
					"start_date":             {Type: "string", Description: "Start date YYYY-MM-DD"},
					"due_date":               {Type: "string", Description: "Due date YYYY-MM-DD"},
				},
				Required: []string{"project", "title"},
			},
		},
		{
			Name:        "track_task_get",
			Description: "Get full details for a task",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id": {Type: "string", Description: "Task ID: PREFIX-NNN or ULID"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_task_move",
			Description: "Move a task to a new status",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":     {Type: "string", Description: "Task ID: PREFIX-NNN or ULID"},
					"status": {Type: "string", Description: "todo | in_progress | blocked | waiting_review | waiting_external | waiting_dependency | done | cancelled"},
				},
				Required: []string{"id", "status"},
			},
		},
		{
			Name:        "track_task_done",
			Description: "Mark a task as done (optionally record actual hours + a completion note)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":           {Type: "string", Description: "Task ID: PREFIX-NNN or ULID"},
					"actual_hours": {Type: "number", Description: "Actual hours spent"},
					"note":         {Type: "string", Description: "Completion note (what shipped / outcome)"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_task_cancel",
			Description: "Cancel a task (terminal, not completed) with an optional reason",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":     {Type: "string", Description: "Task ID: PREFIX-NNN or ULID"},
					"reason": {Type: "string", Description: "Why it's cancelled (stored in completion_note)"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_task_next",
			Description: "Suggest the next task to work on (highest priority unblocked todo)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project": {Type: "string", Description: "Project prefix"},
				},
			},
		},
		{
			Name:        "track_task_link",
			Description: "Create a dependency between two tasks (from_id blocks to_id)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"from_id": {Type: "string", Description: "Task that blocks (PREFIX-NNN or ULID)"},
					"to_id":   {Type: "string", Description: "Task that is blocked (PREFIX-NNN or ULID)"},
					"type":    {Type: "string", Description: "blocks | soft | informational"},
					"reason":  {Type: "string", Description: "Reason for the dependency"},
				},
				Required: []string{"from_id", "to_id"},
			},
		},
		{
			Name:        "track_task_unlink",
			Description: "Remove a dependency between two tasks (the from_id blocks to_id link created by track_task_link)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"from_id": {Type: "string", Description: "Task that blocks (PREFIX-NNN or ULID)"},
					"to_id":   {Type: "string", Description: "Task that is blocked (PREFIX-NNN or ULID)"},
				},
				Required: []string{"from_id", "to_id"},
			},
		},
		{
			Name:        "track_task_delete",
			Description: "Delete a task and all associated data (history, deps, time entries)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id": {Type: "string", Description: "Task ID: PREFIX-NNN or ULID"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_task_update",
			Description: "Update task fields (title, description, type, priority, estimates, due_date, tags, parent_id, sort_order)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":                     {Type: "string", Description: "Task ID: PREFIX-NNN or ULID (required)"},
					"title":                  {Type: "string", Description: "New title"},
					"description":            {Type: "string", Description: "New description"},
					"type":                   {Type: "string", Description: "epic | feature | task"},
					"priority":               {Type: "string", Description: "urgent | high | medium | low"},
					"estimate_size":          {Type: "string", Description: "T-shirt size: XS | S | M | L | XL"},
					"estimate_hours":         {Type: "number", Description: "Estimated hours"},
					"estimate_agent_minutes": {Type: "number", Description: "Estimated agent minutes"},
					"start_date":             {Type: "string", Description: "Start date YYYY-MM-DD"},
					"due_date":               {Type: "string", Description: "Due date YYYY-MM-DD"},
					"tags":                   {Type: "string", Description: "Comma-separated tags"},
					"parent_id":              {Type: "string", Description: "Parent task ID (PREFIX-NNN or ULID), or 'null' to unparent"},
					"sort_order":             {Type: "number", Description: "Sort order within parent"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_session_start",
			Description: "Start a work session for a project",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project": {Type: "string", Description: "Project prefix (required)"},
					"branch":  {Type: "string", Description: "Git branch name"},
				},
				Required: []string{"project"},
			},
		},
		{
			Name:        "track_session_end",
			Description: "End the current active session",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project": {Type: "string", Description: "Project prefix (optional — if omitted, ends most recent session across all projects)"},
					"summary": {Type: "string", Description: "Session summary"},
				},
			},
		},
		{
			Name:        "track_session_log",
			Description: "Log time to a task within the current session",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"task_id": {Type: "string", Description: "Task ID: PREFIX-NNN or ULID"},
					"hours":   {Type: "number", Description: "Hours spent"},
					"note":    {Type: "string", Description: "Note about the work done"},
				},
				Required: []string{"task_id"},
			},
		},
		{
			Name:        "track_session_current",
			Description: "Get the currently active session",
			InputSchema: InputSchema{Type: "object"},
		},
		{
			Name:        "track_decision_create",
			Description: "Record an architectural or technical decision",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":    {Type: "string", Description: "Project prefix (required)"},
					"title":      {Type: "string", Description: "Decision title (required)"},
					"context":    {Type: "string", Description: "Context and problem statement"},
					"revisit_by": {Type: "string", Description: "Date to revisit YYYY-MM-DD"},
				},
				Required: []string{"project", "title"},
			},
		},
		{
			Name:        "track_decision_resolve",
			Description: "Resolve an open decision with a chosen option and rationale",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":        {Type: "string", Description: "Decision ID"},
					"decision":  {Type: "string", Description: "The chosen option"},
					"rationale": {Type: "string", Description: "Why this option was chosen"},
				},
				Required: []string{"id", "decision", "rationale"},
			},
		},
		{
			Name:        "track_learn",
			Description: "Record a learning or pattern discovered during work",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":    {Type: "string", Description: "Project prefix (required)"},
					"title":      {Type: "string", Description: "Learning title (required)"},
					"body":       {Type: "string", Description: "Full explanation (required)"},
					"category":   {Type: "string", Description: "pattern | pitfall | technique | reference"},
					"applies_to": {Type: "array", Description: "Project prefixes this learning applies to", Items: &ItemSchema{Type: "string"}},
				},
				Required: []string{"project", "title", "body"},
			},
		},
		{
			Name:        "track_learn_search",
			Description: "Search learnings by keyword",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"query": {Type: "string", Description: "Search query"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "track_status",
			Description: "Get a project status summary with task counts by status",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project": {Type: "string", Description: "Project prefix (omit for all projects)"},
				},
			},
		},
		{
			Name:        "track_blocker_list",
			Description: "List blockers for a project",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project": {Type: "string", Description: "Project prefix (omit for all projects)"},
					"open":    {Type: "boolean", Description: "Only show open (unresolved) blockers (default true)"},
				},
			},
		},
		{
			Name:        "track_blocker_create",
			Description: "Create a blocker for a project",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":         {Type: "string", Description: "Project prefix (required)"},
					"title":           {Type: "string", Description: "Blocker title (required)"},
					"type":            {Type: "string", Description: "Blocker type (default: external)"},
					"task":            {Type: "string", Description: "Related task ID (PREFIX-NNN or ULID)"},
					"owner":           {Type: "string", Description: "Owner"},
					"escalation_date": {Type: "string", Description: "Escalation date YYYY-MM-DD"},
					"notes":           {Type: "string", Description: "Notes"},
				},
				Required: []string{"project", "title"},
			},
		},
		{
			Name:        "track_blocker_resolve",
			Description: "Resolve (close) a blocker by its id",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id": {Type: "string", Description: "Full blocker id (from track_blocker_list)"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_decision_list",
			Description: "List decisions, optionally filtered by project, status, or expiring",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":  {Type: "string", Description: "Project prefix (omit for all projects)"},
					"status":   {Type: "string", Description: "Comma-separated statuses: pending,decided"},
					"expiring": {Type: "boolean", Description: "Only decisions due for revisit"},
				},
			},
		},
		{
			Name:        "track_decision_update",
			Description: "Edit a decision; only the fields you pass change. (Use track_decision_resolve to set the decision/rationale.)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":         {Type: "string", Description: "Decision id (required)"},
					"title":      {Type: "string", Description: "New title"},
					"context":    {Type: "string", Description: "New context"},
					"options":    {Type: "string", Description: "New options (JSON array or text)"},
					"revisit_by": {Type: "string", Description: "New revisit date YYYY-MM-DD"},
					"decided_by": {Type: "string", Description: "Who decides"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_learn_list",
			Description: "List learnings, optionally filtered by project or category",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":  {Type: "string", Description: "Project prefix (omit for all projects)"},
					"category": {Type: "string", Description: "Filter by category"},
				},
			},
		},
		{
			Name:        "track_learn_update",
			Description: "Edit a learning; only the fields you pass change.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"id":         {Type: "string", Description: "Learning id (required)"},
					"title":      {Type: "string", Description: "New title"},
					"body":       {Type: "string", Description: "New body"},
					"category":   {Type: "string", Description: "New category"},
					"applies_to": {Type: "string", Description: "Comma-separated project prefixes"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "track_sprint_create",
			Description: "Create a sprint for a project",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"project":    {Type: "string", Description: "Project prefix (required)"},
					"name":       {Type: "string", Description: "Sprint name (required)"},
					"goal":       {Type: "string", Description: "Sprint goal"},
					"start_date": {Type: "string", Description: "Start date YYYY-MM-DD"},
					"end_date":   {Type: "string", Description: "End date YYYY-MM-DD"},
				},
				Required: []string{"project", "name"},
			},
		},
		{
			Name:        "track_sprint_list",
			Description: "List sprints for a project",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertySchema{"project": {Type: "string", Description: "Project prefix (required)"}},
				Required:   []string{"project"},
			},
		},
		{
			Name:        "track_sprint_start",
			Description: "Start a sprint (set status to active)",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertySchema{"id": {Type: "string", Description: "Sprint ID or PREFIX-S-N (required)"}},
				Required:   []string{"id"},
			},
		},
		{
			Name:        "track_sprint_complete",
			Description: "Complete a sprint (set status to completed)",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertySchema{"id": {Type: "string", Description: "Sprint ID or PREFIX-S-N (required)"}},
				Required:   []string{"id"},
			},
		},
		{
			Name:        "track_sprint_add",
			Description: "Add a task to a sprint",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"sprint_id": {Type: "string", Description: "Sprint ID or PREFIX-S-N (required)"},
					"task_id":   {Type: "string", Description: "Task ID: PREFIX-NNN or ULID (required)"},
				},
				Required: []string{"sprint_id", "task_id"},
			},
		},
		{
			Name:        "track_sprint_remove",
			Description: "Remove a task from a sprint",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"sprint_id": {Type: "string", Description: "Sprint ID or PREFIX-S-N (required)"},
					"task_id":   {Type: "string", Description: "Task ID: PREFIX-NNN or ULID (required)"},
				},
				Required: []string{"sprint_id", "task_id"},
			},
		},
		{
			Name:        "track_sprint_tasks",
			Description: "List the tasks in a sprint",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]PropertySchema{"sprint_id": {Type: "string", Description: "Sprint ID or PREFIX-S-N (required)"}},
				Required:   []string{"sprint_id"},
			},
		},
	}
}
