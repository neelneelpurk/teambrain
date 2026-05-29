package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillCatalogLists(t *testing.T) {
	code, stdout, _ := runRoot(t, "--json", "skill", "catalog")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if !strings.Contains(stdout.String(), "code-review") {
		t.Fatalf("catalog should list embedded skills:\n%s", stdout.String())
	}
}

func TestSkillAddInstallsFromLibrary(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runRoot(t, "skill", "add", "debug", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "installed") {
		t.Fatalf("output=%q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "debug", "SKILL.md")); err != nil {
		t.Fatalf("skill not installed: %v", err)
	}
	// Idempotent: a second add reports it's already present.
	_, stdout, _ = runRoot(t, "skill", "add", "debug", "--dir", dir)
	if !strings.Contains(stdout.String(), "already installed") {
		t.Fatalf("second add output=%q", stdout.String())
	}
}

func TestSkillAddUnknownIsUserError(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runRoot(t, "skill", "add", "no-such-skill", "--dir", dir)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no embedded skill") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
