package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RunOnYourOwn/track/internal/db"
)

func newTestServer(t *testing.T) (*httptest.Server, *sql.DB) {
	t.Helper()
	conn := db.OpenTestDB(t)
	mux := http.NewServeMux()
	RegisterRoutes(mux, conn)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, conn
}

func doJSON(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestProjectsEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	// empty list
	resp := doJSON(t, "GET", srv.URL+"/api/projects", "")
	if resp.StatusCode != 200 {
		t.Fatalf("list: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// create requires prefix + name
	resp = doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"","name":""}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("create with empty fields: got %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// create ok
	resp = doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"WEB","name":"Web Platform"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// get unknown → 404
	resp = doJSON(t, "GET", srv.URL+"/api/projects/NOPE", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get unknown: got %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTaskCreateValidationHTTP(t *testing.T) {
	srv, _ := newTestServer(t)
	doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"WEB","name":"W"}`).Body.Close()

	// missing title → 400
	resp := doJSON(t, "POST", srv.URL+"/api/projects/WEB/tasks", `{"description":"x"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing title: got %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// create ok → 201, then GET by id
	resp = doJSON(t, "POST", srv.URL+"/api/projects/WEB/tasks", `{"title":"Login form","priority":"high"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create task: got %d, want 201", resp.StatusCode)
	}
	var task struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&task)
	resp.Body.Close()

	resp = doJSON(t, "GET", srv.URL+"/api/tasks/"+task.ID, "")
	if resp.StatusCode != 200 {
		t.Fatalf("get task: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// unknown task → 404
	resp = doJSON(t, "GET", srv.URL+"/api/tasks/NOSUCHID", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get unknown task: got %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// An invalid project prefix is a client error → 400 (not a 500), while a valid
// one is created → 201.
func TestCreateProjectPrefixValidationHTTP(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"<img src=x onerror=alert(1)>","name":"x"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad prefix: got %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"good1","name":"Good"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("valid prefix: got %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()
}

// M7: updateSprint for a nonexistent id returns 404, not 200 null.
func TestUpdateSprintNotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, "PATCH", srv.URL+"/api/sprints/NOSUCH", `{"status":"active"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update unknown sprint: got %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// M5: oversized request bodies are rejected (not read unbounded into memory).
func TestBodySizeLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	huge := `{"prefix":"WEB","name":"` + strings.Repeat("A", 2<<20) + `"}`
	resp := doJSON(t, "POST", srv.URL+"/api/projects", huge)
	if resp.StatusCode == http.StatusCreated {
		t.Fatalf("oversized body should be rejected, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// M10: learning search is scoped to the requested project.
func TestLearningSearchIsProjectScoped(t *testing.T) {
	srv, conn := newTestServer(t)
	doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"AAA","name":"A"}`).Body.Close()
	doJSON(t, "POST", srv.URL+"/api/projects", `{"prefix":"BBB","name":"B"}`).Body.Close()

	pa, _ := db.GetProjectByPrefix(conn, "AAA")
	pb, _ := db.GetProjectByPrefix(conn, "BBB")
	if _, err := db.CreateLearning(conn, db.CreateLearningOpts{ProjectID: pa.ID, Title: "shared topic A", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateLearning(conn, db.CreateLearningOpts{ProjectID: pb.ID, Title: "shared topic B", Body: "y"}); err != nil {
		t.Fatal(err)
	}

	resp := doJSON(t, "GET", srv.URL+"/api/projects/AAA/learnings?q=shared", "")
	var learnings []map[string]any
	json.NewDecoder(resp.Body).Decode(&learnings)
	resp.Body.Close()

	if len(learnings) != 1 {
		t.Fatalf("expected 1 scoped learning for AAA, got %d (cross-project leak)", len(learnings))
	}
	if pid, _ := learnings[0]["project_id"].(string); pid != pa.ID {
		t.Fatalf("returned learning belongs to wrong project: %v", pid)
	}
}
