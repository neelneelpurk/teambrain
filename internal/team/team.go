// Package team implements the 1:1 binding from a personal vault to its team
// vault. The binding is a single plain field in the personal vault's root
// .teambrain.json — no central registry. Rebinding to a different team requires
// an explicit force, so the link is never changed by accident.
package team

import (
	"path/filepath"
	"strings"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/manifest"
)

// IsRemote reports whether target looks like a git remote rather than a local
// path.
func IsRemote(target string) bool {
	return strings.Contains(target, "://") ||
		strings.HasPrefix(target, "git@") ||
		strings.HasSuffix(target, ".git")
}

// Binding builds a TeamBinding for target (a local path or a git remote),
// stamping boundAt.
func Binding(target, boundAt string) (*manifest.TeamBinding, error) {
	b := &manifest.TeamBinding{BoundAt: boundAt}
	if IsRemote(target) {
		b.Remote = target
		return b, nil
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return nil, exit.Userf("resolve team path %q: %v", target, err)
	}
	b.Path = abs
	return b, nil
}

// Bind sets root's team binding to target. If a different team is already bound,
// it refuses unless force is set; rebinding to the same target is idempotent.
func Bind(root *manifest.Root, target, boundAt string, force bool) error {
	next, err := Binding(target, boundAt)
	if err != nil {
		return err
	}
	if root.IsBound() && !sameTarget(root.Team, next) && !force {
		return exit.Userf("a team vault is already bound (%s)", Describe(root.Team)).
			WithHint("pass --force to rebind to a different team")
	}
	root.Team = next
	return nil
}

// Describe renders a binding for messages.
func Describe(b *manifest.TeamBinding) string {
	if b == nil {
		return "none"
	}
	if b.Remote != "" {
		return b.Remote
	}
	return b.Path
}

func sameTarget(a, b *manifest.TeamBinding) bool {
	return a.Path == b.Path && a.Remote == b.Remote
}
