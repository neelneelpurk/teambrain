// Package vault abstracts read/write access to an Obsidian vault behind the
// Vault interface. The default implementation, FSDirect, operates directly on
// the filesystem and always works; the obsidiancli backend (added later) routes
// the same operations through a running Obsidian for link-preserving moves and
// richer queries. A FakeVault supports fast, hermetic tests in other packages.
package vault

import "errors"

// ErrOutsideVault is returned when a path would escape the vault root. teambrain
// never writes outside the vault it was given.
var ErrOutsideVault = errors.New("path escapes the vault")

// MoveReport summarizes a within-vault move: the links it rewrote and the
// references it left intact for manual review, plus the files it touched.
type MoveReport struct {
	From         string        `json:"from"`
	To           string        `json:"to"`
	Rewrites     []LinkRewrite `json:"rewrites"`
	Issues       []LinkIssue   `json:"issues"`
	FilesTouched []string      `json:"files_touched"`
}

// Vault is the backend-agnostic interface to a single vault. All paths are
// vault-relative and use forward slashes; implementations reject paths that
// escape the root.
type Vault interface {
	// Backend names the implementation, e.g. "fs" or "obsidian".
	Backend() string
	// Root returns the absolute vault root.
	Root() string
	// Read returns the bytes of a note.
	Read(rel string) ([]byte, error)
	// Exists reports whether a path exists within the vault.
	Exists(rel string) (bool, error)
	// Write creates or overwrites a note, creating parent directories.
	Write(rel string, content []byte) error
	// Append appends to a note, creating it if necessary.
	Append(rel string, content []byte) error
	// Remove deletes a file or directory (recursively) within the vault. It is
	// not an error if the path does not exist.
	Remove(rel string) error
	// Move relocates a note within the vault, rewriting supported links across
	// the vault and reporting the rest.
	Move(srcRel, dstRel string) (*MoveReport, error)
	// ListNotes returns the vault-relative paths of all Markdown notes under
	// dirRel, sorted.
	ListNotes(dirRel string) ([]string, error)
}
