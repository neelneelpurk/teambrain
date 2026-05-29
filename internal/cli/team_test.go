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

	if code, _, _ := runRoot(t, "team", "bind", "/teams/alpha", "--vault", personal); code != 0 {
		t.Fatal("first bind should succeed")
	}
	// Rebinding to a different team without --force is refused.
	code, _, stderr := runRoot(t, "team", "bind", "/teams/beta", "--vault", personal)
	if code != 1 {
		t.Fatalf("rebind exit=%d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already bound") {
		t.Fatalf("stderr=%q", stderr.String())
	}
	// With --force it succeeds.
	if code, _, _ := runRoot(t, "team", "bind", "/teams/beta", "--vault", personal, "--force"); code != 0 {
		t.Fatal("forced rebind should succeed")
	}
	root, _ := manifest.LoadRoot(personal)
	if root.Team.Path != "/teams/beta" {
		t.Fatalf("binding = %q, want /teams/beta", root.Team.Path)
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
	if !strings.Contains(stdout.String(), "no team vault bound") {
		t.Fatalf("output=%q", stdout.String())
	}
}

func TestSyncCommandsViaCLI(t *testing.T) {
	base := t.TempDir()
	personal := filepath.Join(base, "personal")
	team := filepath.Join(base, "team")
	for _, args := range [][]string{{"init", personal}, {"team", "init", team}, {"team", "bind", team, "--vault", personal}} {
		if code, _, e := runRoot(t, args...); code != 0 {
			t.Fatalf("setup %v: %s", args, e.String())
		}
	}
	// A note linking to a personal-only note.
	note := "---\ntitle: N\n---\nsee [[private]] and [[adrs/other]]\n"
	if err := os.WriteFile(filepath.Join(personal, "projects", "n.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, _, e := runRoot(t, "create-sync", "projects/n.md:adrs/n.md", "--vault", personal); code != 0 {
		t.Fatalf("create-sync: %s", e.String())
	}
	if _, err := os.Stat(filepath.Join(personal, "_sync", "adrs", "n.md")); err != nil {
		t.Fatalf("not staged: %v", err)
	}

	code, stdout, _ := runRoot(t, "--json", "view-sync", "--vault", personal)
	if code != 0 {
		t.Fatalf("view-sync exit=%d", code)
	}
	if !strings.Contains(stdout.String(), "private") {
		t.Fatalf("view-sync should flag the dangling link:\n%s", stdout.String())
	}

	// Dry-run commit-sync needs no git repo.
	if code, out, _ := runRoot(t, "--dry-run", "commit-sync", "--vault", personal); code != 0 {
		t.Fatalf("commit-sync dry-run exit=%d out=%s", code, out.String())
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
