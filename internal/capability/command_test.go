package capability

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/testutil"
)

func TestNewCommandGoldenAndDiscoverable(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	res, err := s.NewCommand("triage", "Triage the inbox into projects and areas.")
	if err != nil {
		t.Fatalf("NewCommand: %v", err)
	}
	if res.Path != "commands/triage.md" {
		t.Fatalf("path = %q", res.Path)
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "new", "command.golden"),
		readFile(t, filepath.Join(dir, res.Path)))

	items, _ := s.List()
	if it, ok := findItem(items, "triage"); !ok || it.Kind != string(KindCommand) {
		t.Fatalf("command not listed: %+v", items)
	}
}

func TestCheckDriftDetectsTamper(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "personal")
	s, dir := targetStore(t)

	// Import a hook (records a checksum).
	plan, _ := s.PlanImport(ImportOptions{Name: "guard", Kind: KindHook, Sources: []Source{src}})
	if _, err := s.Apply(plan); err != nil {
		t.Fatal(err)
	}

	// No drift right after import.
	drifts, err := s.CheckDrift()
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("expected no drift after import, got %+v", drifts)
	}

	// Tamper with the imported script.
	scriptPath := filepath.Join(dir, "hooks", "guard.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nrm -rf /\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	drifts, err = s.CheckDrift()
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0].Name != "guard" || drifts[0].Reason != "modified" {
		t.Fatalf("expected modified drift for guard, got %+v", drifts)
	}

	// Deleting the file reports it as missing.
	if err := os.Remove(scriptPath); err != nil {
		t.Fatal(err)
	}
	drifts, _ = s.CheckDrift()
	if len(drifts) != 1 || drifts[0].Reason != "missing" {
		t.Fatalf("expected missing drift, got %+v", drifts)
	}
}

func TestCheckDriftNoManifest(t *testing.T) {
	t.Parallel()
	s, _ := targetStore(t)
	drifts, err := s.CheckDrift()
	if err != nil {
		t.Fatal(err)
	}
	if drifts != nil {
		t.Fatalf("expected nil drift when nothing is installed, got %+v", drifts)
	}
}
