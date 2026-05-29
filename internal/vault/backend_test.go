package vault

import (
	"fmt"
	"strings"
	"testing"
)

func captureWarn() (Warn, *[]string) {
	var msgs []string
	w := func(format string, args ...any) { msgs = append(msgs, fmt.Sprintf(format, args...)) }
	return w, &msgs
}

func TestOpenAutoSelectsByDetection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	v, err := Open(BackendAuto, dir, func() bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Backend() != "obsidian" {
		t.Fatalf("auto+detected = %q, want obsidian", v.Backend())
	}

	v, err = Open(BackendAuto, dir, func() bool { return false }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Backend() != "fs" {
		t.Fatalf("auto+undetected = %q, want fs", v.Backend())
	}
}

func TestOpenAutoLogsChoice(t *testing.T) {
	t.Parallel()
	warn, msgs := captureWarn()
	if _, err := Open(BackendAuto, t.TempDir(), func() bool { return true }, warn); err != nil {
		t.Fatal(err)
	}
	if !containsSubstr(*msgs, "obsidian") {
		t.Fatalf("auto should log its choice, got %v", *msgs)
	}
}

func TestOpenExplicitObsidianFallsBackWhenUndetected(t *testing.T) {
	t.Parallel()
	warn, msgs := captureWarn()
	v, err := Open(BackendObsidian, t.TempDir(), func() bool { return false }, warn)
	if err != nil {
		t.Fatal(err)
	}
	if v.Backend() != "fs" {
		t.Fatalf("undetected obsidian should fall back to fs, got %q", v.Backend())
	}
	if !containsSubstr(*msgs, "not found") {
		t.Fatalf("expected a fallback warning, got %v", *msgs)
	}
}

func TestOpenFSWarnsOnDesyncRisk(t *testing.T) {
	t.Parallel()
	warn, msgs := captureWarn()
	v, err := Open(BackendFS, t.TempDir(), func() bool { return true }, warn)
	if err != nil {
		t.Fatal(err)
	}
	if v.Backend() != "fs" {
		t.Fatalf("backend = %q, want fs", v.Backend())
	}
	if !containsSubstr(*msgs, "desync") {
		t.Fatalf("fs+obsidian-detected should warn about desync, got %v", *msgs)
	}
}

func containsSubstr(msgs []string, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}
