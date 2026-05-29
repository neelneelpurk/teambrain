// Package git is teambrain's narrow, safety-first git boundary. It exposes only
// the operations promotion needs and, critically, stages by explicit path only
// — never `git add -A` or `.` — so committing into the team vault can never
// sweep up unrelated working-tree changes.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Git is the interface promotion depends on. All methods take the repository
// directory explicitly so implementations are stateless.
type Git interface {
	// IsRepo reports whether dir is inside a git work tree.
	IsRepo(dir string) bool
	// Add stages exactly the given paths (path-scoped; never the whole tree).
	Add(dir string, paths []string) error
	// Commit records a commit limited to the given paths. Restricting the commit
	// to a pathspec means any other staged changes in a dirty tree are left
	// alone, so promotion never commits unrelated work.
	Commit(dir, message string, paths []string) error
	// Push pushes the current branch to its upstream.
	Push(dir string) error
}

// Shell is the real backend that shells out to the git binary.
type Shell struct {
	run func(dir string, args ...string) (string, error)
}

// NewShell returns a Shell that invokes the system git.
func NewShell() *Shell {
	return &Shell{run: runGit}
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// IsRepo reports whether dir is within a git work tree.
func (s *Shell) IsRepo(dir string) bool {
	out, err := s.run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Add stages exactly the given paths. The "--" guard ensures arguments are
// treated as paths, never options, and nothing beyond them is staged.
func (s *Shell) Add(dir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := s.run(dir, args...)
	return err
}

// Commit records a commit limited to the given paths (a pathspec-scoped commit),
// so other staged changes in a dirty tree are not swept in.
func (s *Shell) Commit(dir, message string, paths []string) error {
	args := []string{"commit", "-m", message}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	_, err := s.run(dir, args...)
	return err
}

// Push pushes the current branch to its upstream.
func (s *Shell) Push(dir string) error {
	_, err := s.run(dir, "push")
	return err
}
