package sync

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/testutil"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// newPromoter wires a personal vault to two team targets (alpha, beta) with
// distinct roots so per-team git routing is observable.
func newPromoter(t *testing.T) (*Promoter, *vault.FakeVault, map[string]*vault.FakeVault, *git.Fake) {
	t.Helper()
	personal := vault.NewFakeVault()
	alpha := vault.NewFakeVaultAt("/teams/alpha")
	beta := vault.NewFakeVaultAt("/teams/beta")
	g := git.NewFake()
	p := NewPromoter(personal, []TeamTarget{
		{Name: "alpha", Vault: alpha},
		{Name: "beta", Vault: beta},
	}, g)
	return p, personal, map[string]*vault.FakeVault{"alpha": alpha, "beta": beta}, g
}

func TestCreateSyncRoutesToMultipleTeams(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)

	// A note tagged for both teams.
	_ = personal.Write("projects/adr.md", []byte("---\ntitle: ADR\nteambrains:\n  - alpha\n  - beta\n---\n# ADR\nbody\n"))

	res, err := p.CreateSync([]string{"projects/adr.md"}, false)
	if err != nil {
		t.Fatalf("CreateSync: %v", err)
	}
	if len(res.Staged) != 2 {
		t.Fatalf("expected the note staged to 2 teams, got %d: %+v", len(res.Staged), res.Staged)
	}
	if ok, _ := personal.Exists("_sync/alpha/projects/adr.md"); !ok {
		t.Error("not staged for alpha")
	}
	if ok, _ := personal.Exists("_sync/beta/projects/adr.md"); !ok {
		t.Error("not staged for beta")
	}
	// Routing metadata is stripped from the promoted copy.
	staged, _ := personal.Read("_sync/alpha/projects/adr.md")
	if got := string(staged); contains(got, "teambrains") {
		t.Errorf("promoted copy should not carry the teambrains property:\n%s", got)
	}
	// Original untouched.
	orig, _ := personal.Read("projects/adr.md")
	if !contains(string(orig), "teambrains") {
		t.Error("original note should keep its teambrains property")
	}
}

func TestCreateSyncScansWhenNoPaths(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("a.md", []byte("---\nteambrains: [alpha]\n---\nA\n"))
	_ = personal.Write("b.md", []byte("---\nteambrains: [beta]\n---\nB\n"))
	_ = personal.Write("untagged.md", []byte("# no property\n"))

	res, err := p.CreateSync(nil, false)
	if err != nil {
		t.Fatalf("CreateSync(scan): %v", err)
	}
	if len(res.Staged) != 2 {
		t.Fatalf("scan should stage the 2 tagged notes, got %d: %+v", len(res.Staged), res.Staged)
	}
}

func TestCreateSyncUnknownTeamWarns(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("n.md", []byte("---\nteambrains: [alpha, ghost]\n---\nN\n"))

	res, err := p.CreateSync([]string{"n.md"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Staged) != 1 || res.Staged[0].Team != "alpha" {
		t.Fatalf("only the bound team should be staged: %+v", res.Staged)
	}
	if len(res.Warnings) != 1 || !contains(res.Warnings[0], "ghost") {
		t.Fatalf("expected a warning about the unbound team, got %v", res.Warnings)
	}
}

func TestCreateSyncExplicitUntaggedWarns(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("n.md", []byte("# no teambrains\n"))

	res, err := p.CreateSync([]string{"n.md"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Staged) != 0 || len(res.Warnings) != 1 {
		t.Fatalf("explicit untagged note should warn and stage nothing: %+v / %v", res.Staged, res.Warnings)
	}
}

func TestCreateSyncDestOverride(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("projects/x.md", []byte("---\nteambrains: [alpha]\nteambrain_dest: adrs/0001.md\n---\nX\n"))

	res, err := p.CreateSync([]string{"projects/x.md"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Staged[0].Dest != "adrs/0001.md" {
		t.Fatalf("dest override = %q, want adrs/0001.md", res.Staged[0].Dest)
	}
	if ok, _ := personal.Exists("_sync/alpha/adrs/0001.md"); !ok {
		t.Error("not staged at the overridden dest")
	}
}

func TestCreateSyncRejectsOutOfBoundsDest(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("n.md", []byte("---\nteambrains: [alpha]\nteambrain_dest: ../escape.md\n---\nN\n"))
	if _, err := p.CreateSync([]string{"n.md"}, false); err == nil {
		t.Fatal("an escaping teambrain_dest should be rejected")
	}
}

// TestViewSyncPerTeamLinkIntegrity is the acceptance for the link gate under 1:n.
func TestViewSyncPerTeamLinkIntegrity(t *testing.T) {
	t.Parallel()
	p, personal, teams, _ := newPromoter(t)

	teams["alpha"].Write("conventions/style.md", []byte("# Style\n"))
	// Targets alpha; links to a teammate's note (ok) and a personal-only note (flagged).
	_ = personal.Write("p.md", []byte("---\nteambrains: [alpha]\n---\nsee [[conventions/style]] and [[secret-diary]]\n"))
	if _, err := p.CreateSync([]string{"p.md"}, false); err != nil {
		t.Fatal(err)
	}

	view, err := p.ViewSync()
	if err != nil {
		t.Fatalf("ViewSync: %v", err)
	}
	if len(view.Teams) != 1 || view.Teams[0].Team != "alpha" {
		t.Fatalf("expected one team view for alpha, got %+v", view.Teams)
	}
	issues := view.Teams[0].LinkIssues
	if len(issues) != 1 || issues[0].Link != "secret-diary" {
		t.Fatalf("expected secret-diary flagged, got %+v", issues)
	}
}

// TestCommitSyncFansOutPerTeam is the acceptance for 1:n commit routing.
func TestCommitSyncFansOutPerTeam(t *testing.T) {
	t.Parallel()
	p, personal, teams, g := newPromoter(t)
	_ = personal.Write("shared.md", []byte("---\nteambrains: [alpha, beta]\n---\n# Shared\n"))
	if _, err := p.CreateSync([]string{"shared.md"}, false); err != nil {
		t.Fatal(err)
	}

	res, err := p.CommitSync(CommitOptions{})
	if err != nil {
		t.Fatalf("CommitSync: %v", err)
	}
	if len(res.Teams) != 2 {
		t.Fatalf("expected commits to 2 teams, got %+v", res.Teams)
	}
	// The note landed in both team vaults.
	if ok, _ := teams["alpha"].Exists("shared.md"); !ok {
		t.Error("alpha missing shared.md")
	}
	if ok, _ := teams["beta"].Exists("shared.md"); !ok {
		t.Error("beta missing shared.md")
	}
	// git staged into both team roots, path-scoped.
	if !containsStr(g.AddDirs, "/teams/alpha") || !containsStr(g.AddDirs, "/teams/beta") {
		t.Fatalf("expected adds to both team roots, got %v", g.AddDirs)
	}
	if !containsStr(g.CommitDirs, "/teams/alpha") || !containsStr(g.CommitDirs, "/teams/beta") {
		t.Fatalf("expected commits to both team roots, got %v", g.CommitDirs)
	}
	// Staging cleared.
	if notes, _ := personal.ListNotes("_sync"); len(notes) != 0 {
		t.Fatalf("_sync should be cleared, still has %v", notes)
	}
}

func TestCommitSyncDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	p, personal, teams, g := newPromoter(t)
	_ = personal.Write("n.md", []byte("---\nteambrains: [alpha]\n---\nN\n"))
	_, _ = p.CreateSync([]string{"n.md"}, false)

	res, err := p.CommitSync(CommitOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Teams) != 1 || len(res.Teams[0].Committed) != 1 {
		t.Fatalf("dry-run should report what it would commit: %+v", res.Teams)
	}
	if ok, _ := teams["alpha"].Exists("n.md"); ok {
		t.Error("dry-run must not write to the team vault")
	}
	if len(g.Commits) != 0 {
		t.Error("dry-run must not commit")
	}
	if ok, _ := personal.Exists("_sync/alpha/n.md"); !ok {
		t.Error("dry-run must not clear staging")
	}
}

func TestCommitSyncMessageGolden(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("a.md", []byte("---\nteambrains: [alpha]\n---\nA\n"))
	_ = personal.Write("b.md", []byte("---\nteambrains: [alpha]\n---\nB\n"))
	_, _ = p.CreateSync(nil, false)

	res, err := p.CommitSync(CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenString(t, filepath.Join("testdata", "commit_message.golden"), res.Teams[0].Message)
}

func TestCommitSyncNothingStaged(t *testing.T) {
	t.Parallel()
	p, _, _, _ := newPromoter(t)
	if _, err := p.CommitSync(CommitOptions{}); err == nil {
		t.Fatal("commit with empty staging should error")
	}
}

// TestCommitSyncLinkGateBlocksUnlessForced is the acceptance for the real link
// gate: a payload that would dangle is refused before any write, and --force is
// the documented override.
func TestCommitSyncLinkGateBlocksUnlessForced(t *testing.T) {
	t.Parallel()
	p, personal, teams, g := newPromoter(t)
	teams["alpha"].Write("conventions/style.md", []byte("# Style\n"))
	// Links to a teammate's note (resolves) and a personal-only note (dangles).
	_ = personal.Write("p.md", []byte("---\nteambrains: [alpha]\n---\nsee [[conventions/style]] and [[secret-diary]]\n"))
	if _, err := p.CreateSync([]string{"p.md"}, false); err != nil {
		t.Fatal(err)
	}

	// Without force the dangling link blocks promotion before anything is written.
	if _, err := p.CommitSync(CommitOptions{}); err == nil {
		t.Fatal("expected the link gate to block promotion")
	}
	if ok, _ := teams["alpha"].Exists("p.md"); ok {
		t.Error("a refused commit must not write to the team vault")
	}
	if len(g.Commits) != 0 {
		t.Error("a refused commit must not commit")
	}
	if ok, _ := personal.Exists("_sync/alpha/p.md"); !ok {
		t.Error("staging should be intact after a refused commit")
	}

	// With force it promotes despite the dangling link.
	if _, err := p.CommitSync(CommitOptions{Force: true}); err != nil {
		t.Fatalf("force should override the gate: %v", err)
	}
	if ok, _ := teams["alpha"].Exists("p.md"); !ok {
		t.Error("forced promotion should write to the team vault")
	}
}

// TestCreateSyncScanClearsStaleStaging is the acceptance for the stale-staging
// fix: a whole-vault re-scan drops the orphaned staged copy of a note that is no
// longer tagged, so commit-sync can't promote ghosts.
func TestCreateSyncScanClearsStaleStaging(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("keep.md", []byte("---\nteambrains: [alpha]\n---\nkeep\n"))
	_ = personal.Write("drop.md", []byte("---\nteambrains: [alpha]\n---\ndrop\n"))
	if _, err := p.CreateSync(nil, false); err != nil {
		t.Fatal(err)
	}

	// Untag drop.md, then re-scan.
	_ = personal.Write("drop.md", []byte("# no longer promoted\n"))
	res, err := p.CreateSync(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := personal.Exists("_sync/alpha/drop.md"); ok {
		t.Error("a re-scan should clear the orphaned staged copy of an untagged note")
	}
	if ok, _ := personal.Exists("_sync/alpha/keep.md"); !ok {
		t.Error("still-tagged notes should remain staged after a re-scan")
	}
	if len(res.Staged) != 1 {
		t.Fatalf("re-scan should stage only the still-tagged note, got %d", len(res.Staged))
	}
}

// TestCreateSyncExplicitIsAdditive confirms explicit paths add to existing
// staging rather than rebuilding it (so the promote-to-team skill can stage
// incrementally).
func TestCreateSyncExplicitIsAdditive(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("a.md", []byte("---\nteambrains: [alpha]\n---\nA\n"))
	_ = personal.Write("b.md", []byte("---\nteambrains: [alpha]\n---\nB\n"))
	if _, err := p.CreateSync([]string{"a.md"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CreateSync([]string{"b.md"}, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"_sync/alpha/a.md", "_sync/alpha/b.md"} {
		if ok, _ := personal.Exists(want); !ok {
			t.Errorf("explicit staging should be additive; missing %s", want)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
