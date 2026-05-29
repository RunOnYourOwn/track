package models

import "time"

type Project struct {
	ID         string    `json:"id"`
	Prefix     string    `json:"prefix"`
	Name       string    `json:"name"`
	Phase      string    `json:"phase,omitempty"`
	PhaseType  string    `json:"phase_type,omitempty"`
	ExternalID string    `json:"external_id,omitempty"`
	Metadata   string    `json:"metadata,omitempty"`
	WIPLimit   int       `json:"wip_limit"`
	TaskSort   string    `json:"task_sort"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Task struct {
	ID                   string     `json:"id"`
	ProjectID            string     `json:"project_id"`
	Seq                  int        `json:"seq"`
	Title                string     `json:"title"`
	Description          string     `json:"description,omitempty"`
	Status               string     `json:"status"`
	Priority             string     `json:"priority"`
	EstimateSize         string     `json:"estimate_size,omitempty"`
	EstimateHours        float64    `json:"estimate_hours,omitempty"`
	EstimateAgentMinutes int        `json:"estimate_agent_minutes,omitempty"`
	ActualHours          float64    `json:"actual_hours,omitempty"`
	Type                 string     `json:"type"`
	ParentID             *string    `json:"parent_id,omitempty"`
	SortOrder            int        `json:"sort_order"`
	SourceType           string     `json:"source_type"`
	AgentContext         string     `json:"agent_context,omitempty"`
	Tags                 string     `json:"tags,omitempty"`
	StartDate            *string    `json:"start_date,omitempty"`
	DueDate              *string    `json:"due_date,omitempty"`
	CompletionNote       *string    `json:"completion_note,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	IsRework             bool       `json:"is_rework"`
}

func (t *Task) DisplayID(prefix string) string {
	return prefix + "-" + itoa(t.Seq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

type Dependency struct {
	FromTaskID string `json:"from_task_id"`
	ToTaskID   string `json:"to_task_id"`
	DepType    string `json:"dep_type"`
	Reason     string `json:"reason,omitempty"`
}

type Session struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Branch    string     `json:"branch,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	TasksJSON string     `json:"tasks_json,omitempty"`
}

type TimeEntry struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	SessionID *string   `json:"session_id,omitempty"`
	Hours     float64   `json:"hours"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskStatusHistory struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"task_id"`
	Status    string     `json:"status"`
	EnteredAt time.Time  `json:"entered_at"`
	ExitedAt  *time.Time `json:"exited_at,omitempty"`
}

type TaskCommit struct {
	TaskID       string    `json:"task_id"`
	CommitHash   string    `json:"commit_hash"`
	Repo         string    `json:"repo"`
	CommittedAt  time.Time `json:"committed_at"`
	Message      string    `json:"message,omitempty"`
	FilesChanged string    `json:"files_changed,omitempty"`
}

type Deploy struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Environment string    `json:"environment"`
	DeployedAt  time.Time `json:"deployed_at"`
	CommitHash  string    `json:"commit_hash"`
	Tag         string    `json:"tag,omitempty"`
	TaskIDs     string    `json:"task_ids,omitempty"`
	TriggeredBy string    `json:"triggered_by"`
}

type Decision struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	TaskID       *string    `json:"task_id,omitempty"`
	Title        string     `json:"title"`
	Context      string     `json:"context,omitempty"`
	Options      string     `json:"options,omitempty"`
	Decision     string     `json:"decision,omitempty"`
	Rationale    string     `json:"rationale,omitempty"`
	DecidedBy    string     `json:"decided_by"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	RevisitBy    *string    `json:"revisit_by,omitempty"`
	Status       string     `json:"status"`
	SupersedesID *string    `json:"supersedes_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Learning struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	TaskID    *string   `json:"task_id,omitempty"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	Category  string    `json:"category"`
	AppliesTo string    `json:"applies_to,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CrossProjectDep struct {
	ID              string    `json:"id"`
	SourceProjectID string    `json:"source_project_id"`
	SourceTaskID    *string   `json:"source_task_id,omitempty"`
	TargetProjectID string    `json:"target_project_id"`
	TargetTaskID    *string   `json:"target_task_id,omitempty"`
	DepType         string    `json:"dep_type"`
	Notes           string    `json:"notes,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Blocker struct {
	ID             string     `json:"id"`
	TaskID         *string    `json:"task_id,omitempty"`
	ProjectID      string     `json:"project_id"`
	Title          string     `json:"title"`
	BlockerType    string     `json:"blocker_type"`
	Owner          string     `json:"owner,omitempty"`
	OpenedAt       time.Time  `json:"opened_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	EscalationDate *string    `json:"escalation_date,omitempty"`
	Notes          string     `json:"notes,omitempty"`
}

type Sprint struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Goal      string    `json:"goal,omitempty"`
	Status    string    `json:"status"`
	StartDate *string   `json:"start_date,omitempty"`
	EndDate   *string   `json:"end_date,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Snapshot struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	TakenAt        time.Time `json:"taken_at"`
	Total          int       `json:"total"`
	Done           int       `json:"done"`
	InProgress     int       `json:"in_progress"`
	Todo           int       `json:"todo"`
	Blocked        int       `json:"blocked"`
	HoursDone      float64   `json:"hours_done"`
	HoursRemaining float64   `json:"hours_remaining"`
	FlowEfficiency float64   `json:"flow_efficiency"`
	ReworkRate     float64   `json:"rework_rate"`
	HealthScore    float64   `json:"health_score"`
}

type TaskActivity struct {
	TaskID               string  `json:"task_id"`
	Title                string  `json:"title"`
	Seq                  int     `json:"seq"`
	Completed            bool    `json:"completed"`
	Touched              bool    `json:"touched"`
	CycleTimeSec         *int64  `json:"cycle_time_seconds,omitempty"`
	EstimateHours        float64 `json:"estimate_hours,omitempty"`
	EstimateAgentMinutes int     `json:"estimate_agent_minutes,omitempty"`
	ActualHours          float64 `json:"actual_hours,omitempty"`
}

type SessionStats struct {
	SessionID      string         `json:"session_id"`
	TotalHours     float64        `json:"total_hours"`
	TasksCompleted int            `json:"tasks_completed"`
	TasksTouched   int            `json:"tasks_touched"`
	CommitCount    int            `json:"commit_count"`
	Tasks          []TaskActivity `json:"tasks"`
	Commits        []TaskCommit   `json:"commits"`
}

type SessionSummary struct {
	SessionID      string  `json:"session_id"`
	TotalHours     float64 `json:"total_hours"`
	TasksCompleted int     `json:"tasks_completed"`
	TasksTouched   int     `json:"tasks_touched"`
	CommitCount    int     `json:"commit_count"`
}
