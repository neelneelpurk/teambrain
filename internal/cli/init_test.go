package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInitPath(t *testing.T) {
	t.Parallel()

	if _, err := resolveInitPath(false, nil); err == nil {
		t.Error("neither --here nor a path should error")
	}
	if _, err := resolveInitPath(true, []string{"x"}); err == nil {
		t.Error("both --here and a path should error")
	}
	if got, err := resolveInitPath(false, []string{"/some/path"}); err != nil || got != "/some/path" {
		t.Errorf("path form = %q,%v", got, err)
	}
	cwd, _ := os.Getwd()
	if got, err := resolveInitPath(true, nil); err != nil || got != cwd {
		t.Errorf("--here = %q,%v, want cwd %q", got, err, cwd)
	}
}

func TestInitCommandCreatesVault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "brain")
	code, stdout, stderr := runRoot(t, "init", dir)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if got := stdout.String(); got == "" {
		t.Fatal("expected human output")
	}
	for _, p := range []string{"CLAUDE.md", ".teambrain.json", ".claude/skills/create-teambrain-skill/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("expected %q to exist: %v", p, err)
		}
	}
}

func TestInitCommandDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dry")
	code, stdout, _ := runRoot(t, "--dry-run", "init", dir)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create files, stat err = %v", err)
	}
	if got := stdout.String(); got == "" {
		t.Fatal("dry-run should still report a plan")
	}
}

func TestInitCommandMissingArgIsUserError(t *testing.T) {
	code, _, stderr := runRoot(t, "init")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected an error message")
	}
}
