package git

import "errors"

// Fake is an in-memory Git for tests. It records calls so tests can assert that
// staging is path-scoped (only the intended paths are added).
type Fake struct {
	// NotRepo, when set, makes IsRepo return false.
	NotRepo bool
	// FailPush, when set, makes Push return an error (no upstream/auth).
	FailPush bool

	Added          [][]string
	Commits        []string
	CommittedPaths [][]string
	Pushes         int
}

// NewFake returns an empty fake that reports directories as repos.
func NewFake() *Fake { return &Fake{} }

// IsRepo reports whether the directory is a repo (true unless NotRepo).
func (f *Fake) IsRepo(string) bool { return !f.NotRepo }

// Add records a path-scoped staging call.
func (f *Fake) Add(_ string, paths []string) error {
	f.Added = append(f.Added, append([]string(nil), paths...))
	return nil
}

// Commit records a commit message and its pathspec.
func (f *Fake) Commit(_, message string, paths []string) error {
	f.Commits = append(f.Commits, message)
	f.CommittedPaths = append(f.CommittedPaths, append([]string(nil), paths...))
	return nil
}

// Push records a push, or fails when FailPush is set.
func (f *Fake) Push(string) error {
	if f.FailPush {
		return errors.New("push failed: no upstream configured")
	}
	f.Pushes++
	return nil
}

// AddedPath reports whether path was staged in any Add call.
func (f *Fake) AddedPath(path string) bool {
	for _, call := range f.Added {
		for _, p := range call {
			if p == path {
				return true
			}
		}
	}
	return false
}
