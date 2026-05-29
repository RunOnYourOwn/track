package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/RunOnYourOwn/track/internal/db"
	"github.com/RunOnYourOwn/track/internal/models"
)

// maxBodyBytes caps request bodies to prevent memory-exhaustion via huge/deeply
// nested JSON on the unauthenticated API.
const maxBodyBytes = 1 << 20 // 1 MiB

// RegisterRoutes wires all /api/* endpoints onto mux.
// It wraps every handler with CORS middleware so the web UI
// can call the server from any origin during local development.
func RegisterRoutes(mux *http.ServeMux, conn *sql.DB) {
	h := &handler{conn: conn}

	// Projects
	mux.HandleFunc("GET /api/projects", cors(h.listProjects))
	mux.HandleFunc("POST /api/projects", cors(h.createProject))
	mux.HandleFunc("GET /api/projects/{prefix}", cors(h.getProject))
	mux.HandleFunc("PATCH /api/projects/{prefix}", cors(h.updateProject))
	mux.HandleFunc("DELETE /api/projects/{prefix}", cors(h.deleteProject))
	mux.HandleFunc("GET /api/projects/{prefix}/tasks", cors(h.listTasks))
	mux.HandleFunc("POST /api/projects/{prefix}/tasks", cors(h.createTask))
	mux.HandleFunc("GET /api/projects/{prefix}/sessions", cors(h.listSessions))
	mux.HandleFunc("GET /api/projects/{prefix}/decisions", cors(h.listDecisions))
	mux.HandleFunc("POST /api/projects/{prefix}/decisions", cors(h.createDecision))
	mux.HandleFunc("GET /api/projects/{prefix}/learnings", cors(h.listLearnings))
	mux.HandleFunc("POST /api/projects/{prefix}/learnings", cors(h.createLearning))
	mux.HandleFunc("POST /api/decisions/{id}/resolve", cors(h.resolveDecision))
	mux.HandleFunc("PATCH /api/decisions/{id}", cors(h.updateDecision))
	mux.HandleFunc("PATCH /api/learnings/{id}", cors(h.updateLearning))
	mux.HandleFunc("GET /api/projects/{prefix}/blockers", cors(h.listBlockers))

	// Tasks
	mux.HandleFunc("GET /api/tasks/{id}", cors(h.getTask))
	mux.HandleFunc("PATCH /api/tasks/{id}", cors(h.updateTask))
	mux.HandleFunc("DELETE /api/tasks/{id}", cors(h.deleteTask))
	mux.HandleFunc("GET /api/tasks/{id}/deps", cors(h.getDeps))
	mux.HandleFunc("POST /api/tasks/{id}/deps", cors(h.createDep))
	mux.HandleFunc("DELETE /api/tasks/{id}/deps/{targetId}", cors(h.deleteDep))

	// Sprints
	mux.HandleFunc("GET /api/projects/{prefix}/sprints", cors(h.listSprints))
	mux.HandleFunc("POST /api/projects/{prefix}/sprints", cors(h.createSprint))
	mux.HandleFunc("PATCH /api/sprints/{id}", cors(h.updateSprint))
	mux.HandleFunc("POST /api/sprints/{id}/tasks/{taskId}", cors(h.addSprintTask))
	mux.HandleFunc("DELETE /api/sprints/{id}/tasks/{taskId}", cors(h.removeSprintTask))
	mux.HandleFunc("GET /api/sprints/{id}/tasks", cors(h.listSprintTasks))

	// Sessions
	mux.HandleFunc("GET /api/sessions/{id}/stats", cors(h.getSessionStats))

	// Dashboard + insights
	mux.HandleFunc("GET /api/dashboard", cors(h.dashboard))
	mux.HandleFunc("GET /api/insights", cors(h.insights))
	mux.HandleFunc("GET /api/projects/{prefix}/graph", cors(h.projectGraph))

	// Handle OPTIONS preflight for all /api/ paths.
	mux.HandleFunc("OPTIONS /api/", func(w http.ResponseWriter, r *http.Request) {
		addCORSHeaders(w)
		w.WriteHeader(http.StatusNoContent)
	})
}

// cors is a middleware that adds CORS headers to every response and
// short-circuits OPTIONS preflight requests.
func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addCORSHeaders(w)
		addSecurityHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next(w, r)
	}
}

func addCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func addSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

// handler holds the shared DB connection for all route handlers.
type handler struct {
	conn *sql.DB
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeServerError logs the real error server-side and returns a generic 500 so
// raw database/driver internals (SQL, table/column names) don't leak to clients.
func writeServerError(w http.ResponseWriter, err error) {
	log.Printf("api: internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

// writeFieldError maps a db.ValidationError (a client mistake, e.g. setting a
// derived field) to 400; anything else is a 500.
func writeFieldError(w http.ResponseWriter, err error) {
	var ve *db.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusBadRequest, ve.Msg)
		return
	}
	writeServerError(w, err)
}

// splitCSV splits a comma-separated query parameter value, dropping blanks.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// coalesceProjects returns an empty slice instead of nil so JSON encodes as [].
func coalesceProjects(s []models.Project) []models.Project {
	if s == nil {
		return []models.Project{}
	}
	return s
}

func coalesceTasks(s []models.Task) []models.Task {
	if s == nil {
		return []models.Task{}
	}
	return s
}

func coalesceSessions(s []models.Session) []models.Session {
	if s == nil {
		return []models.Session{}
	}
	return s
}

func coalesceDecisions(s []models.Decision) []models.Decision {
	if s == nil {
		return []models.Decision{}
	}
	return s
}

func coalesceLearnings(s []models.Learning) []models.Learning {
	if s == nil {
		return []models.Learning{}
	}
	return s
}

func coalesceBlockers(s []models.Blocker) []models.Blocker {
	if s == nil {
		return []models.Blocker{}
	}
	return s
}

func coalesceDeps(s []models.Dependency) []models.Dependency {
	if s == nil {
		return []models.Dependency{}
	}
	return s
}

// --- project handlers ---

func (h *handler) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := db.ListProjects(h.conn)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coalesceProjects(projects))
}

func (h *handler) getProject(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type updateProjectRequest struct {
	WIPLimit *int `json:"wip_limit"`
	// Pointer so an explicit "" can clear the phase (omitted = leave unchanged).
	Phase     *string `json:"phase"`
	Name      string  `json:"name"`
	TaskSort  *string `json:"task_sort"`
	PhaseType *string `json:"phase_type"`
}

func (h *handler) updateProject(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.WIPLimit != nil {
		if err := db.UpdateProjectField(h.conn, p.ID, "wip_limit", fmt.Sprintf("%d", *req.WIPLimit)); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if req.Phase != nil {
		if err := db.UpdateProjectField(h.conn, p.ID, "phase", *req.Phase); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if req.Name != "" {
		if err := db.UpdateProjectField(h.conn, p.ID, "name", req.Name); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if req.TaskSort != nil {
		if !db.ValidTaskSorts[*req.TaskSort] {
			writeError(w, http.StatusBadRequest, "invalid task_sort (expected: priority, manual, created, due)")
			return
		}
		if err := db.UpdateProjectField(h.conn, p.ID, "task_sort", *req.TaskSort); err != nil {
			writeServerError(w, err)
			return
		}
	}
	if req.PhaseType != nil {
		if !db.ValidPhaseTypes[*req.PhaseType] {
			writeError(w, http.StatusBadRequest, "invalid phase_type (expected: discovery, design, build, stabilize, maintain)")
			return
		}
		if err := db.UpdateProjectField(h.conn, p.ID, "phase_type", *req.PhaseType); err != nil {
			writeServerError(w, err)
			return
		}
	}

	updated, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type createProjectRequest struct {
	Prefix     string `json:"prefix"`
	Name       string `json:"name"`
	Phase      string `json:"phase"`
	PhaseType  string `json:"phase_type"`
	ExternalID string `json:"external_id"`
	Metadata   string `json:"metadata"`
	WIPLimit   int    `json:"wip_limit"`
}

func (h *handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Prefix == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "prefix and name are required")
		return
	}

	p, err := db.CreateProject(h.conn, req.Prefix, req.Name, req.Phase, req.PhaseType, req.ExternalID, req.Metadata, req.WIPLimit)
	if err != nil {
		var ve *db.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Msg)
			return
		}
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// deleteProject permanently removes a project and ALL its data (tasks, sprints,
// sessions, decisions, learnings, blockers) via the cascading DeleteProject. The
// caller (UI/agent) is responsible for confirming with the user first.
func (h *handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}
	if err := db.DeleteProject(h.conn, p.ID); err != nil {
		writeServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- task handlers ---

func (h *handler) listTasks(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	opts := db.ListTaskOpts{
		ProjectID: p.ID,
		Status:    splitCSV(r.URL.Query().Get("status")),
		Priority:  splitCSV(r.URL.Query().Get("priority")),
		ParentID:  r.URL.Query().Get("parent_id"),
	}

	tasks, err := db.ListTasks(h.conn, opts)
	if err != nil {
		writeServerError(w, err)
		return
	}

	// Enrich with blocker status (real impediments from blockers table)
	type enrichedTask struct {
		models.Task
		Blocked bool `json:"blocked"`
	}

	openBlockers, _ := db.ListBlockers(h.conn, p.ID, true)
	blockedTaskIDs := make(map[string]bool)
	for _, b := range openBlockers {
		if b.TaskID != nil && *b.TaskID != "" {
			blockedTaskIDs[*b.TaskID] = true
		}
	}

	result := make([]enrichedTask, 0, len(tasks))
	for _, t := range tasks {
		et := enrichedTask{Task: t, Blocked: blockedTaskIDs[t.ID]}
		result = append(result, et)
	}
	writeJSON(w, http.StatusOK, result)
}

type createTaskRequest struct {
	Title         string  `json:"title"`
	Type          string  `json:"type"`
	Description   string  `json:"description"`
	Priority      string  `json:"priority"`
	EstimateSize  string  `json:"estimate_size"`
	EstimateHours float64 `json:"estimate_hours"`
	ParentID      string  `json:"parent_id"`
	SourceType    string  `json:"source_type"`
	AgentContext  string  `json:"agent_context"`
	Tags          string  `json:"tags"`
	StartDate     string  `json:"start_date"`
	DueDate       string  `json:"due_date"`
}

func (h *handler) createTask(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	task, err := db.CreateTask(h.conn, db.CreateTaskOpts{
		ProjectID:     p.ID,
		Title:         req.Title,
		Type:          req.Type,
		Description:   req.Description,
		Priority:      req.Priority,
		EstimateSize:  req.EstimateSize,
		EstimateHours: req.EstimateHours,
		ParentID:      req.ParentID,
		SourceType:    req.SourceType,
		AgentContext:  req.AgentContext,
		Tags:          req.Tags,
		StartDate:     req.StartDate,
		DueDate:       req.DueDate,
	})
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *handler) getTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := db.GetTask(h.conn, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

type updateTaskRequest struct {
	Status      string  `json:"status"`
	ActualHours float64 `json:"actual_hours"`
	ParentID    *string `json:"parent_id"`
	Title       string  `json:"title"`
	Priority    string  `json:"priority"`
	// Pointers so an explicit "" can clear the value (omitted = leave unchanged).
	// Title/Priority/Type stay plain strings: they are required/validated enums
	// where an empty value is invalid, so there's nothing to "clear" to.
	StartDate      *string `json:"start_date"`
	DueDate        *string `json:"due_date"`
	CompletionNote *string `json:"completion_note"`
	Description    *string `json:"description"`
	Type           string  `json:"type"`
	EstimateSize   *string `json:"estimate_size"`
	// Pointers so an explicit 0 can clear the estimate / reset the order
	// (omitted = leave unchanged); a plain value can't distinguish the two.
	EstimateHours    *float64 `json:"estimate_hours"`
	EstimateAgentMin *int     `json:"estimate_agent_minutes"`
	SortOrder        *int     `json:"sort_order"`
	Tags             string   `json:"tags"`
}

func (h *handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := db.GetTask(h.conn, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeServerError(w, err)
		return
	}

	var req updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	note := ""
	if req.CompletionNote != nil {
		note = *req.CompletionNote
	}

	if req.Status != "" {
		switch req.Status {
		case "done":
			if err := db.CompleteTask(h.conn, id, req.ActualHours, note); err != nil {
				writeServerError(w, err)
				return
			}
		case "cancelled":
			if err := db.CancelTask(h.conn, id, note); err != nil {
				writeServerError(w, err)
				return
			}
		default:
			if err := db.MoveTask(h.conn, id, req.Status); err != nil {
				writeFieldError(w, err)
				return
			}
		}
	}

	// Allow editing the note without re-closing (the done/cancelled paths above
	// already set it when a status change accompanies the note).
	if req.CompletionNote != nil && req.Status != "done" && req.Status != "cancelled" {
		if err := db.UpdateTaskField(h.conn, id, "completion_note", *req.CompletionNote); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.ParentID != nil {
		if err := db.SetParentID(h.conn, id, *req.ParentID); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.Title != "" {
		if err := db.UpdateTaskField(h.conn, id, "title", req.Title); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.Priority != "" {
		if err := db.UpdateTaskField(h.conn, id, "priority", req.Priority); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.StartDate != nil {
		if err := db.UpdateTaskField(h.conn, id, "start_date", *req.StartDate); err != nil {
			writeFieldError(w, err)
			return
		}
	}

	if req.DueDate != nil {
		if err := db.UpdateTaskField(h.conn, id, "due_date", *req.DueDate); err != nil {
			writeFieldError(w, err)
			return
		}
	}

	if req.Description != nil {
		if err := db.UpdateTaskField(h.conn, id, "description", *req.Description); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.Type != "" {
		if err := db.UpdateTaskField(h.conn, id, "type", req.Type); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.EstimateSize != nil {
		if err := db.UpdateTaskField(h.conn, id, "estimate_size", *req.EstimateSize); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.EstimateHours != nil {
		if *req.EstimateHours < 0 {
			writeError(w, http.StatusBadRequest, "estimate_hours must be >= 0")
			return
		}
		if err := db.UpdateTaskField(h.conn, id, "estimate_hours", strconv.FormatFloat(*req.EstimateHours, 'f', -1, 64)); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.EstimateAgentMin != nil {
		if *req.EstimateAgentMin < 0 {
			writeError(w, http.StatusBadRequest, "estimate_agent_minutes must be >= 0")
			return
		}
		if err := db.UpdateTaskField(h.conn, id, "estimate_agent_minutes", strconv.Itoa(*req.EstimateAgentMin)); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.SortOrder != nil {
		if err := db.UpdateTaskField(h.conn, id, "sort_order", strconv.Itoa(*req.SortOrder)); err != nil {
			writeServerError(w, err)
			return
		}
	}

	if req.Tags != "" {
		if err := db.UpdateTaskField(h.conn, id, "tags", req.Tags); err != nil {
			writeServerError(w, err)
			return
		}
	}

	task, err := db.GetTask(h.conn, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := db.DeleteTask(h.conn, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// --- dependency handlers ---

func (h *handler) getDeps(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deps, err := db.GetBlockers(h.conn, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coalesceDeps(deps))
}

type createDepRequest struct {
	ToTaskID string `json:"to_task_id"`
	DepType  string `json:"dep_type"`
	Reason   string `json:"reason"`
}

func (h *handler) createDep(w http.ResponseWriter, r *http.Request) {
	fromID := r.PathValue("id")

	var req createDepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ToTaskID == "" {
		writeError(w, http.StatusBadRequest, "to_task_id is required")
		return
	}

	if err := db.CreateDependency(h.conn, fromID, req.ToTaskID, req.DepType, req.Reason); err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, models.Dependency{
		FromTaskID: fromID,
		ToTaskID:   req.ToTaskID,
		DepType:    req.DepType,
		Reason:     req.Reason,
	})
}

func (h *handler) deleteDep(w http.ResponseWriter, r *http.Request) {
	fromID := r.PathValue("id")
	toID := r.PathValue("targetId")

	if err := db.DeleteDependency(h.conn, fromID, toID); err != nil {
		writeServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- session / knowledge / blocker handlers ---

type enrichedSession struct {
	models.Session
	Stats *models.SessionSummary `json:"stats,omitempty"`
}

func (h *handler) listSessions(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	sessions, err := db.ListSessions(h.conn, p.ID, limit)
	if err != nil {
		writeServerError(w, err)
		return
	}

	if sessions == nil {
		sessions = []models.Session{}
	}

	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
	}

	batch, _ := db.GetSessionStatsBatch(h.conn, ids)

	enriched := make([]enrichedSession, len(sessions))
	for i, s := range sessions {
		enriched[i] = enrichedSession{Session: s}
		if summary, ok := batch[s.ID]; ok {
			enriched[i].Stats = &summary
		}
	}

	writeJSON(w, http.StatusOK, enriched)
}

func (h *handler) getSessionStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stats, err := db.GetSessionStats(h.conn, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if stats == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *handler) listDecisions(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	statuses := splitCSV(r.URL.Query().Get("status"))
	expiring := r.URL.Query().Get("expiring") == "true"

	decisions, err := db.ListDecisions(h.conn, p.ID, statuses, expiring)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coalesceDecisions(decisions))
}

func (h *handler) listLearnings(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	category := r.URL.Query().Get("category")
	q := r.URL.Query().Get("q")

	var learnings []models.Learning
	var fetchErr error
	if q != "" {
		learnings, fetchErr = db.SearchLearnings(h.conn, p.ID, q)
	} else {
		learnings, fetchErr = db.ListLearnings(h.conn, p.ID, category)
	}
	if fetchErr != nil {
		writeServerError(w, fetchErr)
		return
	}
	writeJSON(w, http.StatusOK, coalesceLearnings(learnings))
}

type createDecisionRequest struct {
	Title     string   `json:"title"`
	Context   string   `json:"context"`
	Options   []string `json:"options"`
	DecidedBy string   `json:"decided_by"`
	RevisitBy string   `json:"revisit_by"`
}

func (h *handler) createDecision(w http.ResponseWriter, r *http.Request) {
	p, err := db.GetProjectByPrefix(h.conn, r.PathValue("prefix"))
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}
	var req createDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	opts := db.CreateDecisionOpts{
		ProjectID: p.ID,
		Title:     req.Title,
		Context:   req.Context,
		DecidedBy: req.DecidedBy,
		RevisitBy: req.RevisitBy,
	}
	if len(req.Options) > 0 {
		if b, err := json.Marshal(req.Options); err == nil {
			opts.Options = string(b)
		}
	}
	dec, err := db.CreateDecision(h.conn, opts)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dec)
}

type createLearningRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Category  string   `json:"category"`
	AppliesTo []string `json:"applies_to"`
}

func (h *handler) createLearning(w http.ResponseWriter, r *http.Request) {
	p, err := db.GetProjectByPrefix(h.conn, r.PathValue("prefix"))
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}
	var req createLearningRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	opts := db.CreateLearningOpts{
		ProjectID: p.ID,
		Title:     req.Title,
		Body:      req.Body,
		Category:  req.Category,
	}
	if len(req.AppliesTo) > 0 {
		if b, err := json.Marshal(req.AppliesTo); err == nil {
			opts.AppliesTo = string(b)
		}
	}
	l, err := db.CreateLearning(h.conn, opts)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

type resolveDecisionRequest struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}

func (h *handler) resolveDecision(w http.ResponseWriter, r *http.Request) {
	var req resolveDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Decision) == "" {
		writeError(w, http.StatusBadRequest, "decision is required")
		return
	}
	if err := db.ResolveDecision(h.conn, r.PathValue("id"), req.Decision, req.Rationale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "decision not found")
			return
		}
		if errors.Is(err, db.ErrDecisionAlreadyDecided) {
			writeError(w, http.StatusConflict, "decision is already decided")
			return
		}
		writeServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateDecisionRequest struct {
	Title     *string `json:"title"`
	Context   *string `json:"context"`
	Options   *string `json:"options"`
	RevisitBy *string `json:"revisit_by"`
	DecidedBy *string `json:"decided_by"`
}

func (h *handler) updateDecision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	set := func(field string, v *string) bool {
		if v == nil {
			return true
		}
		if err := db.UpdateDecisionField(h.conn, id, field, *v); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "decision not found")
			} else {
				writeServerError(w, err)
			}
			return false
		}
		return true
	}
	if !set("title", req.Title) || !set("context", req.Context) || !set("options", req.Options) ||
		!set("revisit_by", req.RevisitBy) || !set("decided_by", req.DecidedBy) {
		return
	}
	updated, err := db.GetDecision(h.conn, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "decision not found")
			return
		}
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type updateLearningRequest struct {
	Title     *string `json:"title"`
	Body      *string `json:"body"`
	Category  *string `json:"category"`
	AppliesTo *string `json:"applies_to"`
}

func (h *handler) updateLearning(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateLearningRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	set := func(field string, v *string) bool {
		if v == nil {
			return true
		}
		if err := db.UpdateLearningField(h.conn, id, field, *v); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "learning not found")
			} else {
				writeServerError(w, err)
			}
			return false
		}
		return true
	}
	if !set("title", req.Title) || !set("body", req.Body) ||
		!set("category", req.Category) || !set("applies_to", req.AppliesTo) {
		return
	}
	updated, err := db.GetLearning(h.conn, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "learning not found")
			return
		}
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *handler) listBlockers(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	openOnly := r.URL.Query().Get("open") == "true"

	blockers, err := db.ListBlockers(h.conn, p.ID, openOnly)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coalesceBlockers(blockers))
}

// --- dashboard ---

type taskCounts struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	InProgress int `json:"in_progress"`
	Todo       int `json:"todo"`
	Waiting    int `json:"waiting"`
	Blocked    int `json:"blocked"`
}

type dashboardProject struct {
	Prefix      string     `json:"prefix"`
	Name        string     `json:"name"`
	Phase       string     `json:"phase"`
	Counts      taskCounts `json:"counts"`
	HealthScore int        `json:"health_score"`
}

type dashboardResponse struct {
	Projects []dashboardProject `json:"projects"`
}

func (h *handler) dashboard(w http.ResponseWriter, r *http.Request) {
	projects, err := db.ListProjects(h.conn)
	if err != nil {
		writeServerError(w, err)
		return
	}

	result := dashboardResponse{Projects: make([]dashboardProject, 0, len(projects))}

	for _, p := range projects {
		tasks, err := db.ListTasks(h.conn, db.ListTaskOpts{ProjectID: p.ID})
		if err != nil {
			writeServerError(w, err)
			return
		}

		openBlockers, err := db.ListBlockers(h.conn, p.ID, true)
		if err != nil {
			writeServerError(w, err)
			return
		}

		var counts taskCounts
		for _, t := range tasks {
			counts.Total++
			switch {
			case t.Status == "done":
				counts.Done++
			case t.Status == "in_progress":
				counts.InProgress++
			case strings.HasPrefix(t.Status, "waiting_"):
				counts.Waiting++
			default:
				counts.Todo++
			}
		}
		counts.Blocked = len(openBlockers)

		proj := p
		score, _ := db.ComputeHealth(&proj, tasks, p.Prefix)

		result.Projects = append(result.Projects, dashboardProject{
			Prefix:      p.Prefix,
			Name:        p.Name,
			Phase:       p.Phase,
			Counts:      counts,
			HealthScore: score,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *handler) insights(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			days = n
		}
	}
	data, err := db.ComputeInsights(h.conn, days)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if data == nil {
		data = []db.ProjectInsights{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *handler) projectGraph(w http.ResponseWriter, r *http.Request) {
	p, err := db.GetProjectByPrefix(h.conn, r.PathValue("prefix"))
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}
	includeDone := r.URL.Query().Get("include_done") == "true"
	g, err := db.ComputeGraph(h.conn, p.ID, includeDone)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if g.Nodes == nil {
		g.Nodes = []db.GraphNode{}
	}
	if g.Edges == nil {
		g.Edges = []db.GraphEdge{}
	}
	writeJSON(w, http.StatusOK, g)
}

// --- sprint handlers ---

func coalesceSprints(s []models.Sprint) []models.Sprint {
	if s == nil {
		return []models.Sprint{}
	}
	return s
}

func (h *handler) listSprints(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}
	sprints, err := db.ListSprints(h.conn, p.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coalesceSprints(sprints))
}

type createSprintRequest struct {
	Name      string `json:"name"`
	Goal      string `json:"goal"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func (h *handler) createSprint(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("prefix")
	p, err := db.GetProjectByPrefix(h.conn, prefix)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeServerError(w, err)
		return
	}

	var req createSprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	sprint, err := db.CreateSprint(h.conn, db.CreateSprintOpts{
		ProjectID: p.ID,
		Name:      req.Name,
		Goal:      req.Goal,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	})
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sprint)
}

type updateSprintRequest struct {
	Status string `json:"status"`
}

func (h *handler) updateSprint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateSprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}
	if !db.ValidSprintStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status (expected: planned, active, completed)")
		return
	}
	if _, err := db.GetSprint(h.conn, id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "sprint not found")
			return
		}
		writeServerError(w, err)
		return
	}
	if err := db.UpdateSprintStatus(h.conn, id, req.Status); err != nil {
		writeServerError(w, err)
		return
	}
	sprint, err := db.GetSprint(h.conn, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sprint)
}

func (h *handler) addSprintTask(w http.ResponseWriter, r *http.Request) {
	sprintID := r.PathValue("id")
	taskID := r.PathValue("taskId")
	if !h.sprintAndTaskExist(w, sprintID, taskID) {
		return
	}
	if err := db.AddTaskToSprint(h.conn, sprintID, taskID); err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *handler) removeSprintTask(w http.ResponseWriter, r *http.Request) {
	sprintID := r.PathValue("id")
	taskID := r.PathValue("taskId")
	if !h.sprintAndTaskExist(w, sprintID, taskID) {
		return
	}
	if err := db.RemoveTaskFromSprint(h.conn, sprintID, taskID); err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// sprintAndTaskExist verifies both ids resolve to real rows, writing a 404 (or a
// 500 on an unexpected error) and returning false if either is missing. The DB
// add/remove use INSERT OR IGNORE / DELETE which silently succeed on bad ids, so
// the existence check has to happen here for the API to return a meaningful code.
func (h *handler) sprintAndTaskExist(w http.ResponseWriter, sprintID, taskID string) bool {
	if _, err := db.GetSprint(h.conn, sprintID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "sprint not found")
			return false
		}
		writeServerError(w, err)
		return false
	}
	if _, err := db.GetTask(h.conn, taskID); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "task not found")
			return false
		}
		writeServerError(w, err)
		return false
	}
	return true
}

func (h *handler) listSprintTasks(w http.ResponseWriter, r *http.Request) {
	sprintID := r.PathValue("id")
	tasks, err := db.ListSprintTasks(h.conn, sprintID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coalesceTasks(tasks))
}
