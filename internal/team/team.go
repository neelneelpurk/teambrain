// Package team implements the 1:n bindings from a personal vault to its team
// vaults. Each binding is a named entry in the personal vault's root
// .teambrain.json — no central registry. Notes route to one or more teams by
// the names they list in their `teambrains:` frontmatter property. Rebinding a
// name to a different target requires an explicit force, so a link is never
// changed by accident.
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

// DeriveName proposes a team name from a target: the repo name for a remote, or
// the final path segment for a local path.
func DeriveName(target string) string {
	if IsRemote(target) {
		base := target
		if i := strings.LastIndexAny(base, "/:"); i >= 0 {
			base = base[i+1:]
		}
		return strings.TrimSuffix(base, ".git")
	}
	return filepath.Base(filepath.Clean(target))
}

// Binding builds a named TeamBinding for target (a local path or a git remote),
// stamping boundAt.
func Binding(name, target, boundAt string) (manifest.TeamBinding, error) {
	b := manifest.TeamBinding{Name: name, BoundAt: boundAt}
	if IsRemote(target) {
		b.Remote = target
		return b, nil
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return manifest.TeamBinding{}, exit.Userf("resolve team path %q: %v", target, err)
	}
	b.Path = abs
	return b, nil
}

// Bind adds or updates the named team binding on root. Rebinding an existing
// name to a different target requires force; rebinding to the same target is
// idempotent. Distinct names coexist (1:n).
func Bind(root *manifest.Root, name, target, boundAt string, force bool) error {
	if name == "" {
		return exit.Userf("a team name is required").WithHint("pass --name <name>")
	}
	next, err := Binding(name, target, boundAt)
	if err != nil {
		return err
	}
	if existing, ok := root.Team(name); ok && !sameTarget(existing, next) && !force {
		return exit.Userf("team %q is already bound to %s", name, Describe(existing)).
			WithHint("pass --force to rebind it to a different target")
	}
	root.UpsertTeam(next)
	return nil
}

// Unbind removes the named team binding.
func Unbind(root *manifest.Root, name string) error {
	if !root.RemoveTeam(name) {
		return exit.Userf("no team named %q is bound", name).
			WithHint("run `teambrain team status` to list bound teams")
	}
	return nil
}

// Describe renders a binding's target for messages.
func Describe(b manifest.TeamBinding) string {
	switch {
	case b.Remote != "":
		return b.Remote
	case b.Path != "":
		return b.Path
	default:
		return b.Name
	}
}

func sameTarget(a, b manifest.TeamBinding) bool {
	return a.Path == b.Path && a.Remote == b.Remote
}
