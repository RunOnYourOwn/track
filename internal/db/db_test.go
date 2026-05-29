package db

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "track-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	db, err := sql.Open("sqlite", tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := configurePragmas(db); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrationsRejectsOutOfOrder(t *testing.T) {
	d := testDB(t)
	if err := runMigrations(d, []migration{{version: 2}, {version: 1}}); err == nil {
		t.Fatal("expected out-of-order migration versions to be rejected")
	}
	if err := runMigrations(d, []migration{{version: 1}, {version: 1}}); err == nil {
		t.Fatal("expected duplicate migration versions to be rejected")
	}
	if err := runMigrations(d, []migration{{version: 1}, {version: 2}}); err != nil {
		t.Fatalf("strictly-increasing versions should pass: %v", err)
	}
}
