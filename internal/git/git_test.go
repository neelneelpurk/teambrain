package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFakeRecordsPathScopedCalls(t *testing.T) {
	t.Parallel()
	f := NewFake()

	if !f.IsRepo("/anywhere") {
		t.Fatal("fake should report repos by default")
	}
	if err := f.Add("/repo", []string{"adrs/1.md", "adrs/2.md"}); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("/repo", "promote: 2 notes", []string{"adrs/1.md", "adrs/2.md"}); err != nil {
		t.Fatal(err)
	}
	if err := f.Push("/repo"); err != nil {
		t.Fatal(err)
	}

	if len(f.Added) != 1 || len(f.Added[0]) != 2 {
		t.Fatalf("expected one Add of two paths, got %v", f.Added)
	}
	if !f.AddedPath("adrs/1.md") || f.AddedPath("nope.md") {
		t.Fatalf("AddedPath wrong: %v", f.Added)
	}
	if len(f.Commits) != 1 || f.Commits[0] != "promote: 2 notes" {
		t.Fatalf("commits = %v", f.Commits)
	}
	if f.Pushes != 1 {
		t.Fatalf("pushes = %d", f.Pushes)
	}
}

func TestFakePushFailure(t *testing.T) {
	t.Parallel()
	f := NewFake()
	f.FailPush = true
	if err := f.Push("/repo"); err == nil {
		t.Fatal("expected push to fail when FailPush is set")
	}
}

// TestShellIntegration exercises the real git backend against a temp repo and a
// local bare remote. It is the single integration test for the git feature.
func TestShellIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Parallel()

	work := t.TempDir()
	repo := filepath.Join(work, "repo")
	bare := filepath.Join(work, "remote.git")

	mustGit(t, "", "init", "--bare", bare)
	mustGit(t, "", "init", repo)
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	mustGit(t, repo, "config", "commit.gpgsign", "false")
	mustGit(t, repo, "remote", "add", "origin", bare)

	g := NewShell()
	if !g.IsRepo(repo) {
		t.Fatal("IsRepo should be true for an initialized repo")
	}
	if g.IsRepo(work) {
		t.Fatal("IsRepo should be false for a non-repo dir")
	}

	// Two files; stage only one, commit, and confirm the other is untouched.
	write(t, filepath.Join(repo, "tracked.md"), "promote me")
	write(t, filepath.Join(repo, "untouched.md"), "leave me")

	if err := g.Add(repo, []string{"tracked.md"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Commit(repo, "promote: tracked", []string{"tracked.md"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	staged := gitOut(t, repo, "show", "--name-only", "--pretty=format:", "HEAD")
	if !strings.Contains(staged, "tracked.md") {
		t.Errorf("commit should include tracked.md, got %q", staged)
	}
	if strings.Contains(staged, "untouched.md") {
		t.Errorf("commit must not include untouched.md (path-scoped staging), got %q", staged)
	}

	// Determine the current branch and push it.
	branch := strings.TrimSpace(gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	mustGit(t, repo, "push", "-u", "origin", branch)
	if err := g.Push(repo); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
