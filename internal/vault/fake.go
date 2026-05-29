package vault

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// FakeVault is an in-memory Vault for hermetic tests in other packages. It
// mirrors FSDirect's observable behavior — including containment checks and
// link-rewriting moves — without touching disk.
type FakeVault struct {
	root  string
	files map[string][]byte
}

// NewFakeVault returns an empty in-memory vault with a default root.
func NewFakeVault() *FakeVault {
	return NewFakeVaultAt("/fake-vault")
}

// NewFakeVaultAt returns an empty in-memory vault with the given root, so tests
// that juggle several vaults can distinguish them by Root().
func NewFakeVaultAt(root string) *FakeVault {
	return &FakeVault{root: root, files: make(map[string][]byte)}
}

// Backend identifies this implementation.
func (f *FakeVault) Backend() string { return "fake" }

// Root returns the synthetic root path.
func (f *FakeVault) Root() string { return f.root }

func (f *FakeVault) key(rel string) (string, error) {
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("%w: %s", ErrOutsideVault, rel)
	}
	return clean, nil
}

// Read returns the bytes of a note.
func (f *FakeVault) Read(rel string) ([]byte, error) {
	k, err := f.key(rel)
	if err != nil {
		return nil, err
	}
	b, ok := f.files[k]
	if !ok {
		return nil, fmt.Errorf("not found: %s", rel)
	}
	return append([]byte(nil), b...), nil
}

// Exists reports whether a note exists.
func (f *FakeVault) Exists(rel string) (bool, error) {
	k, err := f.key(rel)
	if err != nil {
		return false, err
	}
	_, ok := f.files[k]
	return ok, nil
}

// Write creates or overwrites a note.
func (f *FakeVault) Write(rel string, content []byte) error {
	k, err := f.key(rel)
	if err != nil {
		return err
	}
	f.files[k] = append([]byte(nil), content...)
	return nil
}

// Append appends to a note, creating it if necessary.
func (f *FakeVault) Append(rel string, content []byte) error {
	k, err := f.key(rel)
	if err != nil {
		return err
	}
	f.files[k] = append(f.files[k], content...)
	return nil
}

// Remove deletes a file or, when rel names a directory prefix, every file under
// it.
func (f *FakeVault) Remove(rel string) error {
	k, err := f.key(rel)
	if err != nil {
		return err
	}
	delete(f.files, k)
	prefix := k + "/"
	for existing := range f.files {
		if strings.HasPrefix(existing, prefix) {
			delete(f.files, existing)
		}
	}
	return nil
}

// ListNotes returns the Markdown notes under dirRel, sorted.
func (f *FakeVault) ListNotes(dirRel string) ([]string, error) {
	prefix := ""
	if c := path.Clean(filepath.ToSlash(dirRel)); c != "." && c != "" {
		prefix = c + "/"
	}
	var notes []string
	for k := range f.files {
		if strings.HasPrefix(k, prefix) && strings.EqualFold(path.Ext(k), ".md") {
			notes = append(notes, k)
		}
	}
	sort.Strings(notes)
	return notes, nil
}

// Move relocates a note and rewrites supported links across the vault.
func (f *FakeVault) Move(srcRel, dstRel string) (*MoveReport, error) {
	src, err := f.key(srcRel)
	if err != nil {
		return nil, err
	}
	dst, err := f.key(dstRel)
	if err != nil {
		return nil, err
	}
	if _, ok := f.files[src]; !ok {
		return nil, fmt.Errorf("move source not found: %s", srcRel)
	}
	if src != dst {
		if _, ok := f.files[dst]; ok {
			return nil, fmt.Errorf("move destination already exists: %s", dstRel)
		}
	}

	f.files[dst] = f.files[src]
	delete(f.files, src)

	oldTarget := strings.TrimSuffix(src, ".md")
	newTarget := strings.TrimSuffix(dst, ".md")
	report := &MoveReport{From: oldTarget, To: newTarget}

	notes, _ := f.ListNotes(".")
	for _, note := range notes {
		out, rewrites, issues := rewriteLinksForMove(f.files[note], oldTarget, newTarget)
		for i := range issues {
			issues[i].Link = note + ": " + issues[i].Link
		}
		report.Issues = append(report.Issues, issues...)
		if len(rewrites) == 0 {
			continue
		}
		report.Rewrites = append(report.Rewrites, rewrites...)
		report.FilesTouched = append(report.FilesTouched, note)
		f.files[note] = out
	}
	return report, nil
}
