package cmd

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/RunOnYourOwn/track/internal/db"
)

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

	conn, _ := db.Open()
	task, err := db.GetTaskByDisplayID(conn, parts[0], seq)
	if err != nil {
		return "", fmt.Errorf("task %q not found", displayID)
	}
	return task.ID, nil
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
