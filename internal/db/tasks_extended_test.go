package db

import (
	"testing"
)

func TestGetTaskByDisplayID(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "DIS", "Display ID Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "First"})

	got, err := GetTaskByDisplayID(db, "DIS", task.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != task.ID {
		t.Errorf("expected task %s, got %s", task.ID, got.ID)
	}

	// Case insensitive prefix
	got2, err := GetTaskByDisplayID(db, "dis", task.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if got2.ID != task.ID {
		t.Error("case insensitive lookup failed")
	}

	// Not found
	_, err = GetTaskByDisplayID(db, "DIS", 999)
	if err == nil {
		t.Error("expected error for non-existent display ID")
	}
}

func TestListTasksFilters(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "FLT", "Filter Test", "build", "build", "", "{}", 3)
	CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "T1", Priority: "high", Type: "feature"})
	CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "T2", Priority: "low", Type: "task"})
	t3, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "T3", Priority: "high", Type: "task"})
	MoveTask(db, t3.ID, "in_progress")

	// Filter by status
	inProg, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID, Status: []string{"in_progress"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inProg) != 1 {
		t.Errorf("expected 1 in_progress task, got %d", len(inProg))
	}

	// Filter by priority
	high, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID, Priority: []string{"high"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(high) != 2 {
		t.Errorf("expected 2 high priority tasks, got %d", len(high))
	}

	// Filter by type
	features, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID, Type: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if len(features) != 1 {
		t.Errorf("expected 1 feature, got %d", len(features))
	}

	// Multiple status filter
	todoAndIP, err := ListTasks(db, ListTaskOpts{ProjectID: p.ID, Status: []string{"todo", "in_progress"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(todoAndIP) != 3 {
		t.Errorf("expected 3 todo+in_progress, got %d", len(todoAndIP))
	}
}

func TestSetParentID(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "PAR", "Parent Test", "build", "build", "", "{}", 3)
	parent, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Parent", Type: "feature"})
	child, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Child"})
	grandchild, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Grandchild"})

	// Set parent
	if err := SetParentID(db, child.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := GetTask(db, child.ID)
	if got.ParentID == nil || *got.ParentID != parent.ID {
		t.Error("parent not set")
	}

	// Set grandchild parent to child
	if err := SetParentID(db, grandchild.ID, child.ID); err != nil {
		t.Fatal(err)
	}

	// Self-reference should fail
	err := SetParentID(db, child.ID, child.ID)
	if err == nil {
		t.Error("expected error for self-parent")
	}

	// Circular reference: parent -> child -> grandchild, then try to set parent's parent to grandchild
	err = SetParentID(db, parent.ID, grandchild.ID)
	if err == nil {
		t.Error("expected error for circular parent")
	}

	// Clear parent
	if err := SetParentID(db, child.ID, ""); err != nil {
		t.Fatal(err)
	}
	cleared, _ := GetTask(db, child.ID)
	if cleared.ParentID != nil {
		t.Error("expected nil parent_id after clear")
	}
}

func TestUpdateTaskField(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "UPD", "Update Test", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Update me"})

	// Update title
	if err := UpdateTaskField(db, task.ID, "title", "New title"); err != nil {
		t.Fatal(err)
	}
	got, _ := GetTask(db, task.ID)
	if got.Title != "New title" {
		t.Errorf("expected 'New title', got %s", got.Title)
	}

	// Update priority (valid)
	if err := UpdateTaskField(db, task.ID, "priority", "high"); err != nil {
		t.Fatal(err)
	}
	got2, _ := GetTask(db, task.ID)
	if got2.Priority != "high" {
		t.Errorf("expected priority high, got %s", got2.Priority)
	}

	// Update priority (invalid)
	err := UpdateTaskField(db, task.ID, "priority", "super")
	if err == nil {
		t.Error("expected error for invalid priority")
	}

	// Update type (valid)
	if err := UpdateTaskField(db, task.ID, "type", "bug"); err != nil {
		t.Fatal(err)
	}

	// Update type (invalid)
	err = UpdateTaskField(db, task.ID, "type", "invalid_type")
	if err == nil {
		t.Error("expected error for invalid type")
	}

	// Disallowed field
	err = UpdateTaskField(db, task.ID, "status", "done")
	if err == nil {
		t.Error("expected error for disallowed field 'status'")
	}

	// Another disallowed field
	err = UpdateTaskField(db, task.ID, "id", "new-id")
	if err == nil {
		t.Error("expected error for disallowed field 'id'")
	}
}

func TestDeleteTask(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "DEL", "Delete Test", "build", "build", "", "{}", 3)
	parent, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Parent", Type: "feature"})
	child, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Child", ParentID: parent.ID})

	// Add a dependency
	CreateDependency(db, parent.ID, child.ID, "blocks", "needs parent first")

	// Log time on child
	LogTime(db, child.ID, "", 1.0, "work")

	// Record commit on child
	RecordCommit(db, child.ID, "abc123", "track", "2026-05-27T10:00:00Z", "fix", []string{"main.go"})

	// Delete child — should clean up all related rows
	if err := DeleteTask(db, child.ID); err != nil {
		t.Fatal(err)
	}

	// Task should be gone
	_, err := GetTask(db, child.ID)
	if err == nil {
		t.Error("expected error after delete")
	}

	// Parent should no longer list this child
	children, _ := ListTasks(db, ListTaskOpts{ProjectID: p.ID, ParentID: parent.ID})
	if len(children) != 0 {
		t.Errorf("expected 0 children after delete, got %d", len(children))
	}

	// Delete parent — child's parent_id was already set to NULL in delete transaction
	if err := DeleteTask(db, parent.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDependencies(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "DEP", "Dependency Test", "build", "build", "", "{}", 3)
	t1, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Blocker task"})
	t2, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Blocked task"})

	// Create dependency
	if err := CreateDependency(db, t1.ID, t2.ID, "blocks", "needs t1 done first"); err != nil {
		t.Fatal(err)
	}

	// Duplicate should be ignored (INSERT OR IGNORE)
	if err := CreateDependency(db, t1.ID, t2.ID, "blocks", "duplicate"); err != nil {
		t.Fatal(err)
	}

	// GetBlockers should return the dependency
	blockers, err := GetBlockers(db, t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blockers))
	}
	if blockers[0].FromTaskID != t1.ID {
		t.Error("wrong from_task_id")
	}
	if blockers[0].Reason != "needs t1 done first" {
		t.Errorf("expected reason, got %s", blockers[0].Reason)
	}

	// GetActiveBlockers should return it (t1 is still todo)
	active, err := GetActiveBlockers(db, t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active blocker, got %d", len(active))
	}

	// Complete the blocker
	MoveTask(db, t1.ID, "done")

	// Now active blockers should be empty
	active2, err := GetActiveBlockers(db, t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active2) != 0 {
		t.Errorf("expected 0 active blockers after done, got %d", len(active2))
	}

	// GetBlockers still returns it (historical)
	all, _ := GetBlockers(db, t2.ID)
	if len(all) != 1 {
		t.Errorf("expected 1 total blocker, got %d", len(all))
	}

	// Delete dependency
	if err := DeleteDependency(db, t1.ID, t2.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := GetBlockers(db, t2.ID)
	if len(after) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(after))
	}
}

func TestCreateDependencyDefaultType(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "DDF", "Dep Default", "build", "build", "", "{}", 3)
	t1, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "T1"})
	t2, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "T2"})

	// Empty dep_type should default to "blocks"
	if err := CreateDependency(db, t1.ID, t2.ID, "", ""); err != nil {
		t.Fatal(err)
	}

	blockers, _ := GetBlockers(db, t2.ID)
	if len(blockers) != 1 {
		t.Fatal("expected 1 blocker")
	}
	if blockers[0].DepType != "blocks" {
		t.Errorf("expected dep_type 'blocks', got %s", blockers[0].DepType)
	}
}

func TestSuggestNext(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "SGT", "Suggest Test", "build", "build", "", "{}", 3)

	// Empty project — should return nil
	s, err := SuggestNext(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Error("expected nil for empty project")
	}

	// Create tasks with different priorities
	low, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Low", Priority: "low"})
	high, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "High", Priority: "high"})
	urgent, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Urgent", Priority: "urgent"})

	// Should suggest urgent first
	next, err := SuggestNext(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != urgent.ID {
		t.Errorf("expected urgent task, got %v", next)
	}

	// Move urgent to done — should suggest high next
	MoveTask(db, urgent.ID, "done")
	next2, _ := SuggestNext(db, p.ID)
	if next2 == nil || next2.ID != high.ID {
		t.Error("expected high priority after urgent is done")
	}

	// Block high with a dependency on low (which is not done)
	CreateDependency(db, low.ID, high.ID, "blocks", "")

	// Now high is blocked, so suggest low
	next3, _ := SuggestNext(db, p.ID)
	if next3 == nil || next3.ID != low.ID {
		t.Error("expected low task since high is blocked")
	}

	// Move low to done — now high is unblocked
	MoveTask(db, low.ID, "done")
	next4, _ := SuggestNext(db, p.ID)
	if next4 == nil || next4.ID != high.ID {
		t.Error("expected high after blocker is done")
	}
}

func TestMoveTaskInvalidStatus(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "INV", "Invalid", "build", "build", "", "{}", 3)
	task, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "T1"})

	err := MoveTask(db, task.ID, "invalid_status")
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestAutoCloseParent(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "ACP", "AutoClose Parent", "build", "build", "", "{}", 3)
	parent, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Feature", Type: "feature"})
	child1, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Child 1", ParentID: parent.ID})
	child2, _ := CreateTask(db, CreateTaskOpts{ProjectID: p.ID, Title: "Child 2", ParentID: parent.ID})

	// Complete child1 — parent should still be open
	MoveTask(db, child1.ID, "done")
	parentAfter, _ := GetTask(db, parent.ID)
	if parentAfter.Status == "done" {
		t.Error("parent should not auto-close with one child still open")
	}

	// Complete child2 — parent should auto-close
	MoveTask(db, child2.ID, "done")
	parentFinal, _ := GetTask(db, parent.ID)
	if parentFinal.Status != "done" {
		t.Errorf("expected parent to auto-close, got status %s", parentFinal.Status)
	}
}
