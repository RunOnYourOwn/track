package db

import (
	"testing"
)

func TestGetProjectByPrefix(t *testing.T) {
	db := testDB(t)

	p, err := CreateProject(db, "PRJ", "My Project", "build", "build", "", "{}", 3)
	if err != nil {
		t.Fatal(err)
	}

	got, err := GetProjectByPrefix(db, "PRJ")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID {
		t.Errorf("expected ID %s, got %s", p.ID, got.ID)
	}
	if got.Prefix != "PRJ" {
		t.Errorf("expected prefix PRJ, got %s", got.Prefix)
	}

	// Case-insensitive lookup
	got2, err := GetProjectByPrefix(db, "prj")
	if err != nil {
		t.Fatal(err)
	}
	if got2.ID != p.ID {
		t.Errorf("case-insensitive lookup failed")
	}

	// Not found
	_, err = GetProjectByPrefix(db, "NOPE")
	if err == nil {
		t.Fatal("expected error for nonexistent prefix")
	}
}

func TestListProjects(t *testing.T) {
	db := testDB(t)

	CreateProject(db, "AAA", "Alpha", "build", "build", "", "{}", 3)
	CreateProject(db, "BBB", "Beta", "active", "maintain", "ri.test", `{"key":"val"}`, 5)

	projects, err := ListProjects(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	// Ordered by name
	if projects[0].Name != "Alpha" {
		t.Errorf("expected first project Alpha, got %s", projects[0].Name)
	}
	if projects[1].ExternalID != "ri.test" {
		t.Errorf("expected external_id ri.test, got %s", projects[1].ExternalID)
	}
}

func TestUpdateProjectField(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "UPD", "Updatable", "build", "build", "", "{}", 3)

	// Valid update
	if err := UpdateProjectField(db, p.ID, "name", "NewName"); err != nil {
		t.Fatal(err)
	}
	got, _ := GetProjectByID(db, p.ID)
	if got.Name != "NewName" {
		t.Errorf("expected NewName, got %s", got.Name)
	}

	// Disallowed field
	err := UpdateProjectField(db, p.ID, "id", "hacked")
	if err == nil {
		t.Fatal("expected error for disallowed field")
	}

	err = UpdateProjectField(db, p.ID, "prefix", "HACK")
	if err == nil {
		t.Fatal("expected error for disallowed field 'prefix'")
	}
}

func TestDeleteProject(t *testing.T) {
	db := testDB(t)

	p, _ := CreateProject(db, "DEL", "Delete Me", "build", "build", "", "{}", 3)

	if err := DeleteProject(db, p.ID); err != nil {
		t.Fatal(err)
	}

	_, err := GetProjectByID(db, p.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}
