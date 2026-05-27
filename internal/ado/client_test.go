package ado

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWIQL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type application/json")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WIQLResult{
			WorkItems: []WIQLWorkItem{
				{ID: 10, URL: "http://example.com/10"},
				{ID: 20, URL: "http://example.com/20"},
			},
		})
	}))
	defer server.Close()

	client := &Client{org: "myorg", pat: "mypat", httpCli: server.Client(), baseURL: server.URL}

	result, err := client.RunWIQL("MyProject", "MyTeam", "SELECT [System.Id] FROM WorkItems", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.WorkItems) != 2 {
		t.Fatalf("expected 2 work items, got %d", len(result.WorkItems))
	}
	if result.WorkItems[0].ID != 10 {
		t.Errorf("expected ID 10, got %d", result.WorkItems[0].ID)
	}
}

func TestRunWIQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}

	_, err := client.RunWIQL("P", "T", "bad query", 10)
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestGetWorkItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BatchResult{
			Value: []WorkItem{
				{ID: 1, Rev: 3, Fields: map[string]interface{}{"System.Title": "Item 1"}},
				{ID: 2, Rev: 1, Fields: map[string]interface{}{"System.Title": "Item 2"}},
			},
			Count: 2,
		})
	}))
	defer server.Close()

	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}

	items, err := client.GetWorkItems("Proj", []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Rev != 3 {
		t.Errorf("expected rev 3, got %d", items[0].Rev)
	}
}

func TestGetWorkItemsEmpty(t *testing.T) {
	client := &Client{org: "org", pat: "pat", httpCli: http.DefaultClient, baseURL: "http://unused"}

	items, err := client.GetWorkItems("Proj", []int{})
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Error("expected nil for empty ids")
	}
}

func TestGetWorkItemsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := &Client{org: "org", pat: "pat", httpCli: server.Client(), baseURL: server.URL}

	_, err := client.GetWorkItems("Proj", []int{1})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestJsonString(t *testing.T) {
	result := jsonString(`hello "world"`)
	if result != `"hello \"world\""` {
		t.Errorf("unexpected jsonString result: %s", result)
	}
}

func TestConfigPathAndLoadSave(t *testing.T) {
	// ConfigPath should return something
	p := ConfigPath()
	if p == "" {
		t.Error("ConfigPath returned empty")
	}

	// LoadConfig on non-existent path — error
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error loading non-existent config")
	}

	// Create config and load
	cfgDir := filepath.Join(tmpDir, ".track")
	os.MkdirAll(cfgDir, 0755)
	cfg := &Config{Org: "testorg", PatEnv: "MY_PAT", Email: "a@b.com", Syncs: []SyncConfig{{Project: "P", Team: "T", TrackProject: "TP"}}}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(filepath.Join(cfgDir, "ado.json"), data, 0600)

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Org != "testorg" {
		t.Errorf("expected org testorg, got %s", loaded.Org)
	}
	if loaded.PatEnv != "MY_PAT" {
		t.Errorf("expected pat_env MY_PAT, got %s", loaded.PatEnv)
	}

	// SaveConfig
	cfg.Org = "saved"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := LoadConfig()
	if reloaded.Org != "saved" {
		t.Errorf("expected org 'saved' after save, got %s", reloaded.Org)
	}
}

func TestLoadConfigDefaultPatEnv(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfgDir := filepath.Join(tmpDir, ".track")
	os.MkdirAll(cfgDir, 0755)
	// No pat_env field — should default to TRACK_ADO_PAT
	os.WriteFile(filepath.Join(cfgDir, "ado.json"), []byte(`{"org":"x","email":"a@b.com","syncs":[]}`), 0600)

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PatEnv != "TRACK_ADO_PAT" {
		t.Errorf("expected default pat_env, got %s", loaded.PatEnv)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfgDir := filepath.Join(tmpDir, ".track")
	os.MkdirAll(cfgDir, 0755)
	os.WriteFile(filepath.Join(cfgDir, "ado.json"), []byte(`not json`), 0600)

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error for invalid JSON config")
	}
}
