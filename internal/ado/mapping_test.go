package ado

import (
	"testing"
)

func TestMapStateToStatus(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"New", "todo"},
		{"Backlog", "todo"},
		{"Approved", "todo"},
		{"Prioritised", "todo"},
		{"Prepare", "todo"},
		{"Ready", "todo"},
		{"Committed", "in_progress"},
		{"In Progress", "in_progress"},
		{"Active", "in_progress"},
		{"Validate", "in_progress"},
		{"Finished", "done"},
		{"Done", "done"},
		{"Closed", "done"},
		{"Unknown State", "todo"},
		{"", "todo"},
	}
	for _, tc := range cases {
		got := MapStateToStatus(tc.input)
		if got != tc.want {
			t.Errorf("MapStateToStatus(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMapStatusToState(t *testing.T) {
	cases := []struct {
		input  string
		want   string
		wantOK bool
	}{
		{"in_progress", "In Progress", true},
		{"done", "Done", true},
		{"todo", "Backlog", true},
		{"blocked", "", false},
	}
	for _, tc := range cases {
		got, ok := MapStatusToState(tc.input)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("MapStatusToState(%q) = (%q, %v), want (%q, %v)", tc.input, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestMapWorkItemType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Epic", "epic"},
		{"Feature", "feature"},
		{"Product Backlog Item", "feature"},
		{"User Story", "feature"},
		{"Bug", "task"},
		{"Task", "task"},
		{"Something Else", "task"},
	}
	for _, tc := range cases {
		got := MapWorkItemType(tc.input)
		if got != tc.want {
			t.Errorf("MapWorkItemType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestShouldSkipState(t *testing.T) {
	if !ShouldSkipState("Removed") {
		t.Error("expected Removed to be skipped")
	}
	if ShouldSkipState("Done") {
		t.Error("expected Done NOT to be skipped")
	}
	if ShouldSkipState("") {
		t.Error("expected empty NOT to be skipped")
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"plain text", "plain text"},
		{"<p>hello</p>", "hello"},
		{"<div><b>bold</b> and <i>italic</i></div>", "bold and italic"},
		{"a &amp; b &lt; c &gt; d &quot;e&quot; f&#39;g", "a & b < c > d \"e\" f'g"},
		{"word&nbsp;word", "word word"},
		{"  lots   of   spaces  ", "lots of spaces"},
		{"<br/>line<br/>break", "linebreak"},
	}
	for _, tc := range cases {
		got := StripHTML(tc.input)
		if got != tc.want {
			t.Errorf("StripHTML(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractEmail(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"nil", nil, ""},
		{"string", "user@co.com", "user@co.com"},
		{"map with uniqueName", map[string]interface{}{"uniqueName": "a@b.com", "displayName": "A B"}, "a@b.com"},
		{"map without uniqueName", map[string]interface{}{"displayName": "A B"}, "A B"},
		{"map empty", map[string]interface{}{}, ""},
		{"other type", 42, ""},
	}
	for _, tc := range cases {
		got := ExtractEmail(tc.input)
		if got != tc.want {
			t.Errorf("ExtractEmail(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestExtractParentID(t *testing.T) {
	cases := []struct {
		name      string
		relations []Relation
		want      int
	}{
		{"nil relations", nil, 0},
		{"empty relations", []Relation{}, 0},
		{"no parent relation", []Relation{
			{Rel: "System.LinkTypes.Hierarchy-Forward", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/99"},
		}, 0},
		{"valid parent", []Relation{
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/42"},
		}, 42},
		{"multiple relations picks parent", []Relation{
			{Rel: "System.LinkTypes.Hierarchy-Forward", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/10"},
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/55"},
		}, 55},
		{"invalid URL", []Relation{
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: ""},
		}, 0},
	}
	for _, tc := range cases {
		got := ExtractParentID(tc.relations)
		if got != tc.want {
			t.Errorf("ExtractParentID(%s) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestWiqlEscape(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"normal", "normal"},
		{"it's a test", "it''s a test"},
		{"O'Brien's", "O''Brien''s"},
		{"no quotes", "no quotes"},
	}
	for _, tc := range cases {
		got := WiqlEscape(tc.input)
		if got != tc.want {
			t.Errorf("WiqlEscape(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFieldString(t *testing.T) {
	fields := map[string]interface{}{
		"System.Title": "Hello",
		"System.State": "Active",
		"Numeric":      42,
		"NilField":     nil,
	}

	if got := FieldString(fields, "System.Title"); got != "Hello" {
		t.Errorf("expected Hello, got %q", got)
	}
	if got := FieldString(fields, "Missing"); got != "" {
		t.Errorf("expected empty for missing key, got %q", got)
	}
	if got := FieldString(fields, "Numeric"); got != "" {
		t.Errorf("expected empty for non-string, got %q", got)
	}
	if got := FieldString(fields, "NilField"); got != "" {
		t.Errorf("expected empty for nil, got %q", got)
	}
}
