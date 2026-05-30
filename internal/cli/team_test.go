package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/manifest"
)

func TestTeamInitCreatesTeamVault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "team")
	code, _, stderr := runRoot(t, "team", "init", dir)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	root, err := manifest.LoadRoot(dir)
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.Vault != manifest.RoleTeam {
		t.Fatalf("role = %q, want team", root.Vault)
	}
}

func TestTeamBindStatusRoundTrip(t *testing.T) {
	base := t.TempDir()
	personal := filepath.Join(base, "personal")
	team := filepath.Join(base, "team")

	if code, _, e := runRoot(t, "init", personal); code != 0 {
		t.Fatalf("init personal: %s", e.String())
	}
	if code, _, e := runRoot(t, "team", "init", team); code != 0 {
		t.Fatalf("team init: %s", e.String())
	}
	if code, _, e := runRoot(t, "team", "bind", team, "--vault", personal); code != 0 {
		t.Fatalf("team bind: %s", e.String())
	}

	code, stdout, _ := runRoot(t, "--json", "team", "status", "--vault", personal)
	if code != 0 {
		t.Fatalf("team status exit=%d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["bound"] != true {
		t.Fatalf("status should report bound=true, got %v", data)
	}
}

func TestTeamBindForceGuardViaCLI(t *testing.T) {
	base := t.TempDir()
	personal := filepath.Join(base, "personal")
	if code, _, e := runRoot(t, "init", personal); code != 0 {
		t.Fatalf("init: %s", e.String())
	}

	// Bind a named team.
	if code, _, _ := runRoot(t, "team", "bind", "/teams/alpha", "--name", "x", "--vault", personal); code != 0 {
		t.Fatal("first bind should succeed")
	}
	// Rebinding the SAME name to a different target without --force is refused.
	code, _, stderr := runRoot(t, "team", "bind", "/teams/beta", "--name", "x", "--vault", personal)
	if code != 1 {
		t.Fatalf("rebind exit=%d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already bound") {
		t.Fatalf("stderr=%q", stderr.String())
	}
	// With --force it succeeds.
	if code, _, _ := runRoot(t, "team", "bind", "/teams/beta", "--name", "x", "--vault", personal, "--force"); code != 0 {
		t.Fatal("forced rebind should succeed")
	}
	root, _ := manifest.LoadRoot(personal)
	wantBeta, _ := filepath.Abs("/teams/beta") // absolute, OS-specific on Windows
	if b, _ := root.Team("x"); b.Path != wantBeta {
		t.Fatalf("binding = %q, want %q", b.Path, wantBeta)
	}

	// A DIFFERENT name coexists (1:n) without --force.
	if code, _, _ := runRoot(t, "team", "bind", "/teams/gamma", "--name", "y", "--vault", personal); code != 0 {
		t.Fatal("binding a second, differently-named team should succeed")
	}
	root, _ = manifest.LoadRoot(personal)
	if len(root.Teams) != 2 {
		t.Fatalf("expected 2 coexisting teams, got %d", len(root.Teams))
	}
}

func TestTeamStatusUnbound(t *testing.T) {
	personal := filepath.Join(t.TempDir(), "personal")
	if code, _, _ := runRoot(t, "init", personal); code != 0 {
		t.Fatal("init failed")
	}
	code, stdout, _ := runRoot(t, "team", "status", "--vault", personal)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout.String(), "no team vaults bound") {
		t.Fatalf("output=%q", stdout.String())
	}
}

func TestSyncCommandsViaCLI(t *testing.T) {
	base := t.TempDir()
	personal := filepath.Join(base, "personal")
	team := filepath.Join(base, "team")
	for _, args := range [][]string{{"init", personal}, {"team", "init", team}, {"team", "bind", team, "--name", "eng", "--vault", personal}} {
		if code, _, e := runRoot(t, args...); code != 0 {
			t.Fatalf("setup %v: %s", args, e.String())
		}
	}
	// A note tagged for team "eng" that links to a personal-only note.
	note := "---\ntitle: N\nteambrains: [eng]\n---\nsee [[private]] and [[adrs/other]]\n"
	if err := os.WriteFile(filepath.Join(personal, "projects", "n.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, _, e := runRoot(t, "create-sync", "projects/n.md", "--vault", personal); code != 0 {
		t.Fatalf("create-sync: %s", e.String())
	}
	if _, err := os.Stat(filepath.Join(personal, "_sync", "eng", "projects", "n.md")); err != nil {
		t.Fatalf("not staged under _sync/eng: %v", err)
	}

	code, stdout, _ := runRoot(t, "--json", "view-sync", "--vault", personal)
	if code != 0 {
		t.Fatalf("view-sync exit=%d", code)
	}
	if !strings.Contains(stdout.String(), "private") {
		t.Fatalf("view-sync should flag the dangling link:\n%s", stdout.String())
	}

	// The note's links dangle in the team vault, so commit-sync is refused
	// without --force (exit 1, naming the dangling link).
	code, _, stderr := runRoot(t, "--dry-run", "commit-sync", "--vault", personal)
	if code != 1 {
		t.Fatalf("commit-sync without --force should be blocked, exit=%d", code)
	}
	if !strings.Contains(stderr.String(), "private") {
		t.Fatalf("link-gate error should name the dangling link:\n%s", stderr.String())
	}

	// With --force, dry-run commit-sync needs no git repo and no confirmation.
	if code, out, _ := runRoot(t, "--dry-run", "commit-sync", "--force", "--vault", personal); code != 0 {
		t.Fatalf("forced commit-sync dry-run exit=%d out=%s", code, out.String())
	}
}

// TestCommitSyncConfirmsBeforeWriting locks in that commit-sync — which writes
// to shared team repos — never does so without confirmation. Both the abort and
// the --json paths short-circuit before any git work, so no real repo is needed.
func TestCommitSyncConfirmsBeforeWriting(t *testing.T) {
	base := t.TempDir()
	personal := filepath.Join(base, "personal")
	team := filepath.Join(base, "team")
	for _, args := range [][]string{{"init", personal}, {"team", "init", team}, {"team", "bind", team, "--name", "eng", "--vault", personal}} {
		if code, _, e := runRoot(t, args...); code != 0 {
			t.Fatalf("setup %v: %s", args, e.String())
		}
	}
	// A clean note (no dangling links) tagged for eng.
	note := "---\nteambrains: [eng]\nteambrain_dest: adrs/clean.md\n---\n# Clean\n"
	if err := os.WriteFile(filepath.Join(personal, "projects", "clean.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, e := runRoot(t, "create-sync", "projects/clean.md", "--vault", personal); code != 0 {
		t.Fatalf("create-sync: %s", e.String())
	}

	// Empty stdin + no --yes → abort, write nothing, exit 0.
	code, _, stderr := runRoot(t, "commit-sync", "--vault", personal)
	if code != 0 {
		t.Fatalf("aborted commit-sync should exit 0, got %d (%s)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "aborted") {
		t.Fatalf("expected an abort message:\n%s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(team, "adrs", "clean.md")); !os.IsNotExist(err) {
		t.Fatal("aborted commit-sync must not write to the team vault")
	}

	// Machine mode refuses without --yes rather than blocking on a prompt.
	jcode, jout, _ := runRoot(t, "--json", "commit-sync", "--vault", personal)
	if jcode != 1 {
		t.Fatalf("--json commit-sync without --yes should be a user error, exit=%d", jcode)
	}
	if !strings.Contains(jout.String(), "confirmation") {
		t.Fatalf("--json error should mention confirmation:\n%s", jout.String())
	}
}

func TestSyncNotAVaultIsPreconditionError(t *testing.T) {
	empty := t.TempDir()
	code, _, stderr := runRoot(t, "view-sync", "--vault", empty)
	if code != 2 {
		t.Fatalf("exit=%d, want 2 (precondition)", code)
	}
	if !strings.Contains(stderr.String(), "not a teambrain vault") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
