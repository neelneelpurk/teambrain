package sync

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// TestCommitSyncIntegration runs the full promotion against a real git repo and
// a local bare remote — the single end-to-end git test for promotion.
func TestCommitSyncIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Parallel()

	work := t.TempDir()
	personalDir := filepath.Join(work, "personal")
	teamDir := filepath.Join(work, "team")
	bare := filepath.Join(work, "remote.git")

	run(t, "", "git", "init", "--bare", bare)
	run(t, "", "git", "init", teamDir)
	run(t, teamDir, "git", "config", "user.email", "t@example.com")
	run(t, teamDir, "git", "config", "user.name", "T")
	run(t, teamDir, "git", "config", "commit.gpgsign", "false")
	// Seed an initial commit so a branch exists, then wire the remote.
	run(t, teamDir, "git", "commit", "--allow-empty", "-m", "init")
	run(t, teamDir, "git", "remote", "add", "origin", bare)
	branch := strings.TrimSpace(out(t, teamDir, "git", "rev-parse", "--abbrev-ref", "HEAD"))
	run(t, teamDir, "git", "push", "-u", "origin", branch)

	personal, err := vault.NewFSDirect(personalDir)
	if err != nil {
		t.Fatal(err)
	}
	team, err := vault.NewFSDirect(teamDir)
	if err != nil {
		t.Fatal(err)
	}

	// The note routes itself to team "eng" via its teambrains property, and
	// overrides its destination path.
	if err := personal.Write("projects/decision.md",
		[]byte("---\ntitle: Decision\nteambrains: [eng]\nteambrain_dest: adrs/0001-decision.md\n---\n# Decision\n")); err != nil {
		t.Fatal(err)
	}

	p := NewPromoter(personal, []TeamTarget{{Name: "eng", Vault: team}}, git.NewShell())
	if _, err := p.CreateSync([]string{"projects/decision.md"}, false); err != nil {
		t.Fatalf("CreateSync: %v", err)
	}
	res, err := p.CommitSync(CommitOptions{Push: true})
	if err != nil {
		t.Fatalf("CommitSync: %v", err)
	}
	if len(res.Teams) != 1 || !res.Teams[0].Pushed {
		t.Errorf("expected one pushed team commit, got %+v", res.Teams)
	}

	// The promoted note is committed in the team repo...
	log := out(t, teamDir, "git", "log", "--name-only", "--pretty=format:", "-1")
	if !strings.Contains(log, "adrs/0001-decision.md") {
		t.Errorf("last commit should include the promoted note, got %q", log)
	}
	// ...and present on the bare remote.
	remoteFiles := out(t, "", "git", "--git-dir", bare, "ls-tree", "-r", "--name-only", branch)
	if !strings.Contains(remoteFiles, "adrs/0001-decision.md") {
		t.Errorf("promoted note not found on remote, got %q", remoteFiles)
	}
	// _sync was cleared.
	if ok, _ := personal.Exists("_sync/eng/adrs/0001-decision.md"); ok {
		t.Error("_sync should be cleared after commit-sync")
	}
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, o)
	}
}

func out(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	o, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, o)
	}
	return string(o)
}
