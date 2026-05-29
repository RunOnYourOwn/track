package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/RunOnYourOwn/track/internal/db"
)

// mustOpen opens the database or aborts with a clean message. A DB that won't
// open is fatal for any CLI command, so this replaces the old `conn, _ :=
// db.Open()` pattern that silently swallowed the error and then panicked on a
// nil connection at first use.
func mustOpen() *sql.DB {
	conn, err := db.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "track: cannot open database: %v\n", err)
		os.Exit(1)
	}
	return conn
}

func resolveID(displayID string) (string, error) {
	// If it looks like a ULID (26 chars), use directly
	if len(displayID) == 26 && !strings.Contains(displayID, "-") {
		return displayID, nil
	}

	// Parse PREFIX-NNN
	parts := strings.SplitN(displayID, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid task ID %q (expected PREFIX-NNN)", displayID)
	}

	seq, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid seq in %q", displayID)
	}

	conn := mustOpen()
	task, err := db.GetTaskByDisplayID(conn, parts[0], seq)
	if err != nil {
		return "", fmt.Errorf("task %q not found", displayID)
	}
	return task.ID, nil
}

// escapeLIKE escapes backslash, percent, and underscore in s so it can be used
// as a safe prefix in a SQLite LIKE ? ESCAPE '\' query.
func escapeLIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func getPrefix(conn *sql.DB, projectID, hint string) string {
	if hint != "" && hint != "?" {
		return strings.ToUpper(hint)
	}
	var prefix string
	row := conn.QueryRow("SELECT prefix FROM projects WHERE id = ?", projectID)
	_ = row.Scan(&prefix)
	return prefix
}
