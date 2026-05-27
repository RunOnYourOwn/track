package db

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// OpenTestDB creates a temporary SQLite database with schema applied.
// Intended for use by other packages' tests that need a real DB.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "track-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	d, err := sql.Open("sqlite", tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := configurePragmas(d); err != nil {
		t.Fatal(err)
	}
	if err := migrate(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
