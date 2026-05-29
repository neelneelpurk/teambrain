package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillNewAndList(t *testing.T) {
	dir := t.TempDir()

	code, _, stderr := runRoot(t, "skill", "new", "daily-review", "--description", "Summarize the day", "--dir", dir)
	if code != 0 {
		t.Fatalf("skill new exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "daily-review", "SKILL.md")); err != nil {
		t.Fatalf("skill not written: %v", err)
	}

	code, stdout, _ := runRoot(t, "--json", "skill", "list", "--dir", dir)
	if code != 0 {
		t.Fatalf("skill list exit=%d", code)
	}
	if !strings.Contains(stdout.String(), `"name": "daily-review"`) {
		t.Fatalf("list did not show the skill:\n%s", stdout.String())
	}
}

func TestAgentNewAndList(t *testing.T) {
	dir := t.TempDir()
	if code, _, e := runRoot(t, "agent", "new", "researcher", "--dir", dir); code != 0 {
		t.Fatalf("agent new failed: %s", e.String())
	}
	code, stdout, _ := runRoot(t, "--json", "agent", "list", "--dir", dir)
	if code != 0 || !strings.Contains(stdout.String(), `"researcher"`) {
		t.Fatalf("agent list = %d\n%s", code, stdout.String())
	}
}

func TestHookNewListAndJSON(t *testing.T) {
	dir := t.TempDir()
	code, _, e := runRoot(t, "hook", "new", "fmt", "--event", "PostToolUse", "--matcher", "Edit", "--dir", dir)
	if code != 0 {
		t.Fatalf("hook new failed: %s", e.String())
	}
	for _, p := range []string{".claude/hooks/fmt.sh", ".claude/settings.json", ".claude/.teambrain.json"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("expected %s: %v", p, err)
		}
	}

	code, stdout, _ := runRoot(t, "--json", "hook", "list", "--dir", dir)
	if code != 0 {
		t.Fatalf("hook list exit=%d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if !strings.Contains(stdout.String(), `"event": "PostToolUse"`) {
		t.Errorf("hook list missing event annotation:\n%s", stdout.String())
	}
}

func TestCapabilityListEmpty(t *testing.T) {
	dir := t.TempDir()
	code, stdout, _ := runRoot(t, "skill", "list", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout.String(), "no skills found") {
		t.Fatalf("expected empty message, got %q", stdout.String())
	}
}

func TestSkillNewDryRun(t *testing.T) {
	dir := t.TempDir()
	code, stdout, _ := runRoot(t, "--dry-run", "skill", "new", "ghost", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout.String(), "would create") {
		t.Fatalf("dry-run output = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "ghost", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write")
	}
}

func TestHookNewMissingEventIsUserError(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runRoot(t, "hook", "new", "x", "--dir", dir)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "event") {
		t.Fatalf("error should mention event: %q", stderr.String())
	}
}
