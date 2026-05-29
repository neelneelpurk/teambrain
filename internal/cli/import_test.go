package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/capability"
)

// makeSourceVault creates a .claude under dir/.claude with a skill and a hook.
func makeSourceVault(t *testing.T, dir string) {
	t.Helper()
	store := capability.OpenStore(filepath.Join(dir, ".claude"))
	if _, err := store.NewSkill("shared", "A shared skill"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.NewHook(capability.HookOptions{Name: "guard", Event: "PreToolUse", Matcher: "Bash"}); err != nil {
		t.Fatal(err)
	}
}

func TestImportSkillCommand(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	repo := filepath.Join(base, "repo")
	makeSourceVault(t, vault)

	code, stdout, stderr := runRoot(t, "skill", "import", "shared", "--source", vault, "--dir", repo)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "skills", "shared", "SKILL.md")); err != nil {
		t.Fatalf("skill not imported: %v", err)
	}
	if !strings.Contains(stdout.String(), "imported") {
		t.Errorf("output = %q", stdout.String())
	}
}

func TestImportHookConfirmationAbort(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	repo := filepath.Join(base, "repo")
	makeSourceVault(t, vault)

	// Answer "n" to the confirmation prompt: nothing should be imported.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	io := IO{In: strings.NewReader("n\n"), Out: stdout, Err: stderr}
	code := Execute(io, BuildInfo{Version: "test"},
		[]string{"hook", "import", "guard", "--source", vault, "--dir", repo})
	if code != 0 {
		t.Fatalf("abort should exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "hooks", "guard.sh")); !os.IsNotExist(err) {
		t.Fatal("hook should not be imported when the user declines")
	}
	if !strings.Contains(stderr.String(), "aborted") {
		t.Errorf("expected an abort message, stderr=%q", stderr.String())
	}
}

func TestImportHookConfirmedWithYes(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	repo := filepath.Join(base, "repo")
	makeSourceVault(t, vault)

	code, _, stderr := runRoot(t, "hook", "import", "guard", "--source", vault, "--dir", repo, "--yes")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "hooks", "guard.sh")); err != nil {
		t.Fatalf("hook not imported with --yes: %v", err)
	}
}

func TestUninstallCommand(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	repo := filepath.Join(base, "repo")
	makeSourceVault(t, vault)

	if code, _, e := runRoot(t, "skill", "import", "shared", "--source", vault, "--dir", repo); code != 0 {
		t.Fatalf("import failed: %s", e.String())
	}
	code, _, stderr := runRoot(t, "skill", "uninstall", "shared", "--dir", repo, "--yes")
	if code != 0 {
		t.Fatalf("uninstall exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "skills", "shared")); !os.IsNotExist(err) {
		t.Fatal("skill should be removed after uninstall")
	}
}

func TestImportNoSourceIsUserError(t *testing.T) {
	repo := t.TempDir()
	code, _, stderr := runRoot(t, "skill", "import", "shared", "--dir", repo)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no import sources") {
		t.Errorf("stderr=%q", stderr.String())
	}
}

func TestResolveSourcesFromConfigPersonalVault(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	makeSourceVault(t, vault)

	app := &App{Config: &Config{PersonalVault: vault}}
	srcs, err := resolveSources(app, nil)
	if err != nil {
		t.Fatalf("resolveSources: %v", err)
	}
	if len(srcs) != 1 || srcs[0].Label != "personal" {
		t.Fatalf("sources = %+v, want one personal source", srcs)
	}
	if filepath.Base(srcs[0].ClaudeDir) != ".claude" {
		t.Fatalf("claude dir = %q", srcs[0].ClaudeDir)
	}
}
