package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FSDirect is the default Vault backend. It reads and writes vault files
// directly and is always available, with no dependency on a running Obsidian.
type FSDirect struct {
	root string // absolute, cleaned
}

// NewFSDirect returns a vault rooted at root (which need not exist yet for
// writes that create it). The root is resolved to an absolute path.
func NewFSDirect(root string) (*FSDirect, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &FSDirect{root: filepath.Clean(abs)}, nil
}

// Backend identifies this implementation.
func (v *FSDirect) Backend() string { return "fs" }

// Root returns the absolute vault root.
func (v *FSDirect) Root() string { return v.root }

// resolve joins rel to the root and guarantees the result stays inside the
// vault, returning ErrOutsideVault otherwise.
func (v *FSDirect) resolve(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("%w: empty path", ErrOutsideVault)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	joined := clean
	if !filepath.IsAbs(clean) {
		joined = filepath.Join(v.root, clean)
	}
	joined = filepath.Clean(joined)

	rp, err := filepath.Rel(v.root, joined)
	if err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrOutsideVault, rel)
	}
	return joined, nil
}

// Read returns the bytes of a note.
func (v *FSDirect) Read(rel string) ([]byte, error) {
	abs, err := v.resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// Exists reports whether a path exists within the vault.
func (v *FSDirect) Exists(rel string) (bool, error) {
	abs, err := v.resolve(rel)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(abs)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

// Write creates or overwrites a note, creating parent directories.
func (v *FSDirect) Write(rel string, content []byte) error {
	abs, err := v.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, content, 0o644)
}

// Append appends to a note, creating it if necessary.
func (v *FSDirect) Append(rel string, content []byte) error {
	abs, err := v.resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(content)
	return err
}

// Remove deletes a file or directory (recursively) within the vault.
func (v *FSDirect) Remove(rel string) error {
	abs, err := v.resolve(rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(abs)
}

// ListNotes returns the vault-relative paths of all Markdown notes under dirRel.
func (v *FSDirect) ListNotes(dirRel string) ([]string, error) {
	absDir, err := v.resolve(dirRel)
	if err != nil {
		return nil, err
	}
	var notes []string
	walkErr := filepath.WalkDir(absDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(v.root, p)
		if relErr != nil {
			return relErr
		}
		notes = append(notes, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(notes)
	return notes, nil
}

// Move relocates a note and rewrites supported links across the vault. It
// refuses to overwrite an existing destination.
func (v *FSDirect) Move(srcRel, dstRel string) (*MoveReport, error) {
	srcAbs, err := v.resolve(srcRel)
	if err != nil {
		return nil, err
	}
	dstAbs, err := v.resolve(dstRel)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(srcAbs); err != nil {
		return nil, fmt.Errorf("move source: %w", err)
	}
	if srcAbs != dstAbs {
		if _, err := os.Stat(dstAbs); err == nil {
			return nil, fmt.Errorf("move destination already exists: %s", dstRel)
		}
	}

	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(srcAbs, dstAbs); err != nil {
		return nil, err
	}

	oldTarget := targetFromRel(v.root, srcAbs)
	newTarget := targetFromRel(v.root, dstAbs)

	report := &MoveReport{From: oldTarget, To: newTarget}

	notes, err := v.ListNotes(".")
	if err != nil {
		return report, err
	}
	for _, note := range notes {
		content, readErr := v.Read(note)
		if readErr != nil {
			return report, readErr
		}
		out, rewrites, issues := rewriteLinksForMove(content, oldTarget, newTarget)
		for i := range issues {
			issues[i].Link = note + ": " + issues[i].Link
		}
		report.Issues = append(report.Issues, issues...)
		if len(rewrites) == 0 {
			continue
		}
		report.Rewrites = append(report.Rewrites, rewrites...)
		report.FilesTouched = append(report.FilesTouched, note)
		if writeErr := v.Write(note, out); writeErr != nil {
			return report, writeErr
		}
	}
	return report, nil
}

// targetFromRel converts an absolute note path to its vault-relative link
// target: forward slashes, no ".md" extension.
func targetFromRel(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		rel = abs
	}
	return strings.TrimSuffix(filepath.ToSlash(rel), ".md")
}
