package team

import (
	"path/filepath"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/manifest"
)

func TestIsRemote(t *testing.T) {
	t.Parallel()
	remotes := []string{"git@github.com:org/team.git", "https://github.com/org/team.git", "ssh://host/repo"}
	for _, r := range remotes {
		if !IsRemote(r) {
			t.Errorf("IsRemote(%q) = false, want true", r)
		}
	}
	for _, p := range []string{"/home/u/team", "./team", "team-brain"} {
		if IsRemote(p) {
			t.Errorf("IsRemote(%q) = true, want false", p)
		}
	}
}

func TestBindingPathIsAbsolute(t *testing.T) {
	t.Parallel()
	b, err := Binding("relative/team", "2026-05-29T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(b.Path) {
		t.Fatalf("path binding should be absolute, got %q", b.Path)
	}
	if b.Remote != "" {
		t.Fatalf("path binding should not set remote: %q", b.Remote)
	}
}

func TestBindForceGuard(t *testing.T) {
	t.Parallel()
	root := manifest.NewRoot(manifest.RolePersonal)

	// First bind succeeds.
	if err := Bind(root, "/teams/alpha", "t0", false); err != nil {
		t.Fatalf("first bind: %v", err)
	}
	if !root.IsBound() || root.Team.Path != "/teams/alpha" {
		t.Fatalf("binding not recorded: %+v", root.Team)
	}

	// Rebinding to the SAME target is idempotent (no force needed).
	if err := Bind(root, "/teams/alpha", "t1", false); err != nil {
		t.Fatalf("idempotent rebind should succeed: %v", err)
	}

	// Rebinding to a DIFFERENT target without force is refused.
	if err := Bind(root, "/teams/beta", "t2", false); err == nil {
		t.Fatal("rebinding to a different team without --force should fail")
	}
	if root.Team.Path != "/teams/alpha" {
		t.Fatalf("refused rebind must not change the binding, got %q", root.Team.Path)
	}

	// With force, it rebinds.
	if err := Bind(root, "/teams/beta", "t3", true); err != nil {
		t.Fatalf("forced rebind: %v", err)
	}
	if root.Team.Path != "/teams/beta" {
		t.Fatalf("forced rebind did not take: %+v", root.Team)
	}
}

func TestBindRemote(t *testing.T) {
	t.Parallel()
	root := manifest.NewRoot(manifest.RolePersonal)
	if err := Bind(root, "git@github.com:org/team.git", "t0", false); err != nil {
		t.Fatal(err)
	}
	if root.Team.Remote != "git@github.com:org/team.git" || root.Team.Path != "" {
		t.Fatalf("remote binding wrong: %+v", root.Team)
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	if Describe(nil) != "none" {
		t.Error("nil binding should describe as none")
	}
	if Describe(&manifest.TeamBinding{Path: "/p"}) != "/p" {
		t.Error("path describe")
	}
	if Describe(&manifest.TeamBinding{Remote: "r"}) != "r" {
		t.Error("remote describe")
	}
}
