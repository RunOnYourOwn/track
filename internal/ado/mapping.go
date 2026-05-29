package ado

import (
	"fmt"
	"regexp"
	"strings"
)

var stateToStatus = map[string]string{
	"New":         "todo",
	"Proposed":    "todo",
	"To Do":       "todo",
	"Backlog":     "todo",
	"Approved":    "todo",
	"Prioritised": "todo",
	"Prepare":     "todo",
	"Ready":       "todo",
	"Committed":   "in_progress",
	"In Progress": "in_progress",
	"Doing":       "in_progress",
	"Active":      "in_progress",
	"Validate":    "in_progress",
	"Resolved":    "done",
	"Finished":    "done",
	"Done":        "done",
	"Closed":      "done",
}

var statusToState = map[string]string{
	"todo":        "Backlog",
	"in_progress": "In Progress",
	"done":        "Done",
}

var typeMapping = map[string]string{
	"Epic":                 "epic",
	"Feature":              "feature",
	"Product Backlog Item": "feature",
	"Bug":                  "task",
	"Task":                 "task",
	"User Story":           "feature",
}

func MapStateToStatus(adoState string) string {
	if s, ok := stateToStatus[adoState]; ok {
		return s
	}
	return "todo"
}

func MapStatusToState(trackStatus string) (string, bool) {
	s, ok := statusToState[trackStatus]
	return s, ok
}

func MapWorkItemType(adoType string) string {
	if t, ok := typeMapping[adoType]; ok {
		return t
	}
	return "task"
}

func ShouldSkipState(adoState string) bool {
	return adoState == "Removed"
}

var (
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	spaceRegex   = regexp.MustCompile(`\s+`)
)

func StripHTML(html string) string {
	if html == "" {
		return ""
	}
	s := htmlTagRegex.ReplaceAllString(html, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = spaceRegex.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func ExtractEmail(assignedTo interface{}) string {
	if assignedTo == nil {
		return ""
	}
	switch v := assignedTo.(type) {
	case string:
		return v
	case map[string]interface{}:
		if email, ok := v["uniqueName"].(string); ok {
			return email
		}
		if name, ok := v["displayName"].(string); ok {
			return name
		}
	}
	return ""
}

func ExtractParentID(relations []Relation) int {
	for _, r := range relations {
		if r.Rel == "System.LinkTypes.Hierarchy-Reverse" {
			parts := strings.Split(r.URL, "/")
			if len(parts) > 0 {
				var id int
				if n, _ := fmt.Sscanf(parts[len(parts)-1], "%d", &id); n == 1 && id > 0 {
					return id
				}
			}
		}
	}
	return 0
}

func WiqlEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func FieldString(fields map[string]interface{}, key string) string {
	v, ok := fields[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
