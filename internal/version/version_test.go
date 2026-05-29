package version

import "testing"

func TestStringUsesLdflagWhenSet(t *testing.T) {
	old := Version
	Version = "v9.9.9"
	defer func() { Version = old }()
	if got := String(); got != "v9.9.9" {
		t.Fatalf("String(): got %q want v9.9.9", got)
	}
}

func TestStringFallsBackWhenUnset(t *testing.T) {
	old := Version
	Version = ""
	defer func() { Version = old }()
	// No ldflag + a test binary has no module version, so it must fall back to a
	// non-empty sentinel rather than the empty string.
	if got := String(); got == "" {
		t.Fatal("String() returned empty with no ldflag")
	}
}
