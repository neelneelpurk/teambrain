package team

import (
	"path/filepath"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/manifest"
)

func TestIsRemote(t *testing.T) {
	t.Parallel()
	for _, r := range []string{"git@github.com:org/team.git", "https://github.com/org/team.git", "ssh://host/repo"} {
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

func TestDeriveName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/home/u/team-alpha":               "team-alpha",
		"git@github.com:org/team-beta.git": "team-beta",
		"https://github.com/org/gamma.git": "gamma",
		"./relative/delta":                 "delta",
	}
	for in, want := range cases {
		if got := DeriveName(in); got != want {
			t.Errorf("DeriveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBindManyTeamsCoexist(t *testing.T) {
	t.Parallel()
	root := manifest.NewRoot(manifest.RolePersonal)

	if err := Bind(root, "alpha", "/teams/alpha", "t0", false); err != nil {
		t.Fatal(err)
	}
	if err := Bind(root, "beta", "git@github.com:org/beta.git", "t0", false); err != nil {
		t.Fatal(err)
	}
	if len(root.Teams) != 2 {
		t.Fatalf("expected two coexisting teams, got %d", len(root.Teams))
	}
	if a, ok := root.Team("alpha"); !ok || !filepath.IsAbs(a.Path) {
		t.Fatalf("alpha path should be absolute: %+v", a)
	}
	if b, _ := root.Team("beta"); b.Remote != "git@github.com:org/beta.git" {
		t.Fatalf("beta remote wrong: %+v", b)
	}
}

func TestBindForceGuardPerName(t *testing.T) {
	t.Parallel()
	root := manifest.NewRoot(manifest.RolePersonal)
	// Bind stores absolute paths, which are OS-specific (e.g. D:\teams\alpha on
	// Windows), so compare against the resolved form rather than the literal.
	wantAlpha := mustAbs(t, "/teams/alpha")
	wantOther := mustAbs(t, "/teams/other")

	if err := Bind(root, "alpha", "/teams/alpha", "t0", false); err != nil {
		t.Fatal(err)
	}
	// Same name, same target -> idempotent.
	if err := Bind(root, "alpha", "/teams/alpha", "t1", false); err != nil {
		t.Fatalf("idempotent rebind should succeed: %v", err)
	}
	// Same name, different target, no force -> refused.
	if err := Bind(root, "alpha", "/teams/other", "t2", false); err == nil {
		t.Fatal("rebinding a name to a different target without --force should fail")
	}
	if a, _ := root.Team("alpha"); a.Path != wantAlpha {
		t.Fatalf("refused rebind must not change the target, got %q", a.Path)
	}
	// With force -> rebinds.
	if err := Bind(root, "alpha", "/teams/other", "t3", true); err != nil {
		t.Fatalf("forced rebind: %v", err)
	}
	if a, _ := root.Team("alpha"); a.Path != wantOther {
		t.Fatalf("forced rebind did not take: %+v", a)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs %q: %v", p, err)
	}
	return abs
}

func TestUnbind(t *testing.T) {
	t.Parallel()
	root := manifest.NewRoot(manifest.RolePersonal)
	_ = Bind(root, "alpha", "/teams/alpha", "t0", false)
	if err := Unbind(root, "alpha"); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if root.IsBound() {
		t.Fatal("root should be unbound after removing the only team")
	}
	if err := Unbind(root, "alpha"); err == nil {
		t.Fatal("unbinding a missing team should error")
	}
}

func TestBindRequiresName(t *testing.T) {
	t.Parallel()
	root := manifest.NewRoot(manifest.RolePersonal)
	if err := Bind(root, "", "/teams/x", "t0", false); err == nil {
		t.Fatal("empty team name should be rejected")
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	if Describe(manifest.TeamBinding{Name: "n"}) != "n" {
		t.Error("nameless target should fall back to name")
	}
	if Describe(manifest.TeamBinding{Path: "/p"}) != "/p" {
		t.Error("path describe")
	}
	if Describe(manifest.TeamBinding{Remote: "r"}) != "r" {
		t.Error("remote describe")
	}
}
