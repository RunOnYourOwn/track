package ado

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/RunOnYourOwn/track/internal/db"
)

func TestIsLocalDirty(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name   string
		task   *db.TaskRecord
		want   bool
	}{
		{
			name: "empty agent_context",
			task: &db.TaskRecord{AgentContext: "", UpdatedAt: now},
			want: false,
		},
		{
			name: "invalid JSON",
			task: &db.TaskRecord{AgentContext: "not json", UpdatedAt: now},
			want: false,
		},
		{
			name: "no last_synced_at",
			task: &db.TaskRecord{AgentContext: `{"ado_id":1}`, UpdatedAt: now},
			want: false,
		},
		{
			name: "updated before sync (not dirty)",
			task: &db.TaskRecord{
				AgentContext: mustJSON(AgentContext{AdoID: 1, LastSyncedAt: now.Format(time.RFC3339)}),
				UpdatedAt:    now.Add(-10 * time.Second),
			},
			want: false,
		},
		{
			name: "updated within 5s grace (not dirty)",
			task: &db.TaskRecord{
				AgentContext: mustJSON(AgentContext{AdoID: 1, LastSyncedAt: now.Add(-3 * time.Second).Format(time.RFC3339)}),
				UpdatedAt:    now,
			},
			want: false,
		},
		{
			name: "updated well after sync (dirty)",
			task: &db.TaskRecord{
				AgentContext: mustJSON(AgentContext{AdoID: 1, LastSyncedAt: now.Add(-60 * time.Second).Format(time.RFC3339)}),
				UpdatedAt:    now,
			},
			want: true,
		},
		{
			name: "invalid last_synced_at format",
			task: &db.TaskRecord{
				AgentContext: `{"ado_id":1,"last_synced_at":"not-a-date"}`,
				UpdatedAt:    now,
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isLocalDirty(tc.task)
			if got != tc.want {
				t.Errorf("isLocalDirty(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
