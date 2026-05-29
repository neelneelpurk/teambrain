package sync

import (
	"path/filepath"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/testutil"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

func newPromoter(t *testing.T) (*Promoter, *vault.FakeVault, *vault.FakeVault, *git.Fake) {
	t.Helper()
	personal := vault.NewFakeVault()
	team := vault.NewFakeVault()
	g := git.NewFake()
	return NewPromoter(personal, team, g), personal, team, g
}

func TestCreateSyncStagesAndNormalizes(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)

	_ = personal.Write("projects/adr.md", []byte("---\ntitle: ADR\n---\n# ADR\n\nbody [[other]]\n"))

	res, err := p.CreateSync([]Spec{{Src: "projects/adr.md", Dest: "adrs/0001.md"}}, false)
	if err != nil {
		t.Fatalf("CreateSync: %v", err)
	}
	if len(res.Staged) != 1 || res.Staged[0].Dest != "adrs/0001.md" {
		t.Fatalf("unexpected staged: %+v", res.Staged)
	}

	// Staged under _sync mirroring the team destination.
	if ok, _ := personal.Exists("_sync/adrs/0001.md"); !ok {
		t.Fatal("note not staged under _sync")
	}
	// Original is untouched (copy, not move).
	if ok, _ := personal.Exists("projects/adr.md"); !ok {
		t.Fatal("original note should remain")
	}
}

func TestCreateSyncDefaultsDestToSource(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("conventions/style.md", []byte("# Style\n"))

	res, err := p.CreateSync([]Spec{{Src: "conventions/style.md"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Staged[0].Dest != "conventions/style.md" {
		t.Fatalf("dest = %q, want it to default to src", res.Staged[0].Dest)
	}
}

func TestCreateSyncRejectsOutOfBoundsDest(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("a.md", []byte("x"))
	if _, err := p.CreateSync([]Spec{{Src: "a.md", Dest: "../escape.md"}}, false); err == nil {
		t.Fatal("expected an out-of-bounds destination to be rejected")
	}
}

func TestCreateSyncDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	_ = personal.Write("a.md", []byte("x"))
	if _, err := p.CreateSync([]Spec{{Src: "a.md", Dest: "a.md"}}, true); err != nil {
		t.Fatal(err)
	}
	if ok, _ := personal.Exists("_sync/a.md"); ok {
		t.Fatal("dry-run must not stage")
	}
}

// TestViewSyncLinkIntegrity is the Phase 5 acceptance for the link gate: a link
// to a personal-only note is flagged; a link to another promoted note resolves.
func TestViewSyncLinkIntegrity(t *testing.T) {
	t.Parallel()
	p, personal, team, _ := newPromoter(t)

	// The team already has conventions/style.
	_ = team.Write("conventions/style.md", []byte("# Style\n"))

	// Promote two notes: one links to a teammate's existing note (ok), to the
	// other promoted note (ok), and to a personal-only note (must be flagged).
	_ = personal.Write("projects/a.md", []byte("see [[conventions/style]], [[adrs/0002]], and [[secret-diary]]\n"))
	_ = personal.Write("projects/b.md", []byte("# B\n"))
	if _, err := p.CreateSync([]Spec{
		{Src: "projects/a.md", Dest: "adrs/0001.md"},
		{Src: "projects/b.md", Dest: "adrs/0002.md"},
	}, false); err != nil {
		t.Fatal(err)
	}

	view, err := p.ViewSync()
	if err != nil {
		t.Fatalf("ViewSync: %v", err)
	}

	if len(view.LinkIssues) != 1 {
		t.Fatalf("expected exactly one link issue, got %+v", view.LinkIssues)
	}
	if view.LinkIssues[0].Link != "secret-diary" {
		t.Fatalf("flagged link = %q, want secret-diary", view.LinkIssues[0].Link)
	}

	// Both staged notes appear; adrs/0001 is new to the team.
	var found bool
	for _, it := range view.Items {
		if it.Dest == "adrs/0001.md" {
			found = true
			if it.Status != "new" {
				t.Errorf("status = %q, want new", it.Status)
			}
		}
	}
	if !found {
		t.Fatal("staged note adrs/0001.md not in view")
	}
}

func TestViewSyncDiffGolden(t *testing.T) {
	t.Parallel()
	p, personal, team, _ := newPromoter(t)

	team.Write("notes/x.md", []byte("line one\nline two\nline three\n"))
	personal.Write("src.md", []byte("line one\nline two CHANGED\nline three\nline four\n"))
	if _, err := p.CreateSync([]Spec{{Src: "src.md", Dest: "notes/x.md"}}, false); err != nil {
		t.Fatal(err)
	}
	view, err := p.ViewSync()
	if err != nil {
		t.Fatal(err)
	}
	var diff string
	for _, it := range view.Items {
		if it.Dest == "notes/x.md" {
			if it.Status != "modified" {
				t.Fatalf("status = %q, want modified", it.Status)
			}
			diff = it.Diff
		}
	}
	testutil.AssertGoldenString(t, filepath.Join("testdata", "view_diff.golden"), diff)
}

// TestCommitSyncPathScoped is the Phase 5 acceptance for staging: commit-sync
// touches only the promoted files.
func TestCommitSyncPathScoped(t *testing.T) {
	t.Parallel()
	p, personal, team, g := newPromoter(t)

	personal.Write("a.md", []byte("# A\n"))
	personal.Write("b.md", []byte("# B\n"))
	if _, err := p.CreateSync([]Spec{
		{Src: "a.md", Dest: "adrs/a.md"},
		{Src: "b.md", Dest: "runbooks/b.md"},
	}, false); err != nil {
		t.Fatal(err)
	}

	res, err := p.CommitSync(CommitOptions{})
	if err != nil {
		t.Fatalf("CommitSync: %v", err)
	}

	// Team vault now has both notes.
	if ok, _ := team.Exists("adrs/a.md"); !ok {
		t.Error("team missing adrs/a.md")
	}
	if ok, _ := team.Exists("runbooks/b.md"); !ok {
		t.Error("team missing runbooks/b.md")
	}
	// git staged exactly the promoted paths — nothing else.
	want := []string{"adrs/a.md", "runbooks/b.md"}
	if len(g.Added) != 1 || !equalStringSet(g.Added[0], want) {
		t.Fatalf("staged paths = %v, want exactly %v", g.Added, want)
	}
	if len(g.CommittedPaths) != 1 || !equalStringSet(g.CommittedPaths[0], want) {
		t.Fatalf("committed paths = %v, want %v", g.CommittedPaths, want)
	}
	if len(res.Committed) != 2 {
		t.Fatalf("result committed = %v", res.Committed)
	}
	// _sync is cleared.
	notes, _ := personal.ListNotes("_sync")
	if len(notes) != 0 {
		t.Fatalf("_sync should be cleared, still has %v", notes)
	}
}

func TestCommitSyncMessageGolden(t *testing.T) {
	t.Parallel()
	p, personal, _, _ := newPromoter(t)
	personal.Write("a.md", []byte("# A\n"))
	personal.Write("b.md", []byte("# B\n"))
	p.CreateSync([]Spec{{Src: "a.md", Dest: "adrs/a.md"}, {Src: "b.md", Dest: "runbooks/b.md"}}, false)

	res, err := p.CommitSync(CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenString(t, filepath.Join("testdata", "commit_message.golden"), res.Message)
}

func TestCommitSyncPushAndCustomMessage(t *testing.T) {
	t.Parallel()
	p, personal, _, g := newPromoter(t)
	personal.Write("a.md", []byte("# A\n"))
	p.CreateSync([]Spec{{Src: "a.md", Dest: "adrs/a.md"}}, false)

	res, err := p.CommitSync(CommitOptions{Message: "custom message", Push: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Message != "custom message" {
		t.Errorf("message = %q", res.Message)
	}
	if !res.Pushed || g.Pushes != 1 {
		t.Errorf("expected a push")
	}
}

func TestCommitSyncErrors(t *testing.T) {
	t.Parallel()

	// Nothing staged.
	p, _, _, _ := newPromoter(t)
	if _, err := p.CommitSync(CommitOptions{}); err == nil {
		t.Error("commit with empty _sync should error")
	}

	// No team bound.
	personal := vault.NewFakeVault()
	personal.Write("a.md", []byte("x"))
	p2 := NewPromoter(personal, nil, git.NewFake())
	p2.CreateSync([]Spec{{Src: "a.md", Dest: "a.md"}}, false)
	if _, err := p2.CommitSync(CommitOptions{}); err == nil {
		t.Error("commit with no team bound should error")
	}
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
