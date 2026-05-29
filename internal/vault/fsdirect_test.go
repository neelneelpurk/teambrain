package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestVault(t *testing.T) *FSDirect {
	t.Helper()
	v, err := NewFSDirect(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSDirect: %v", err)
	}
	return v
}

func TestFSDirectRoundTrip(t *testing.T) {
	t.Parallel()
	v := newTestVault(t)

	if ok, _ := v.Exists("notes/a.md"); ok {
		t.Fatal("file should not exist yet")
	}
	if err := v.Write("notes/a.md", []byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ok, err := v.Exists("notes/a.md")
	if err != nil || !ok {
		t.Fatalf("Exists after write = %v,%v", ok, err)
	}
	got, err := v.Read("notes/a.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "hello\n" {
		t.Fatalf("Read = %q", got)
	}
	if err := v.Append("notes/a.md", []byte("world\n")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, _ = v.Read("notes/a.md")
	if string(got) != "hello\nworld\n" {
		t.Fatalf("after append Read = %q", got)
	}
}

func TestFSDirectRefusesWritesOutsideVault(t *testing.T) {
	t.Parallel()
	v := newTestVault(t)

	bad := []string{"../escape.md", "../../etc/passwd", "notes/../../x.md"}
	for _, p := range bad {
		if err := v.Write(p, []byte("x")); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("Write(%q) error = %v, want ErrOutsideVault", p, err)
		}
		if _, err := v.Read(p); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("Read(%q) error = %v, want ErrOutsideVault", p, err)
		}
	}

	// An absolute path pointing outside the vault is also refused.
	if err := v.Write(filepath.Join(t.TempDir(), "other.md"), []byte("x")); !errors.Is(err, ErrOutsideVault) {
		t.Errorf("absolute outside path error = %v, want ErrOutsideVault", err)
	}
}

func TestFSDirectListNotes(t *testing.T) {
	t.Parallel()
	v := newTestVault(t)
	for _, p := range []string{"a.md", "sub/b.md", "sub/deep/c.md", "ignore.txt"} {
		if err := v.Write(p, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	notes, err := v.ListNotes(".")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	want := []string{"a.md", "sub/b.md", "sub/deep/c.md"}
	if len(notes) != len(want) {
		t.Fatalf("ListNotes = %v, want %v", notes, want)
	}
	for i := range want {
		if notes[i] != want[i] {
			t.Fatalf("ListNotes[%d] = %q, want %q", i, notes[i], want[i])
		}
	}
}

// TestFSDirectMoveAcceptance is the Phase 1 acceptance scenario: a within-vault
// move rewrites supported links across the vault and reports the rest.
func TestFSDirectMoveAcceptance(t *testing.T) {
	t.Parallel()
	v := newTestVault(t)

	mustWrite(t, v, "projects/old.md", "---\ntitle: Old\n---\n# Old\n")
	mustWrite(t, v, "daily/note1.md",
		"full [[projects/old]], bare [[old]], embed ![[old]], block [[old#^b1]]\n")
	mustWrite(t, v, "areas/other.md", "unrelated [[areas/health]]\n")

	report, err := v.Move("projects/old.md", "archive/new.md")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	// The file moved.
	if ok, _ := v.Exists("projects/old.md"); ok {
		t.Error("source should no longer exist")
	}
	if ok, _ := v.Exists("archive/new.md"); !ok {
		t.Error("destination should exist")
	}
	if got, _ := v.Read("archive/new.md"); string(got) != "---\ntitle: Old\n---\n# Old\n" {
		t.Errorf("moved content changed: %q", got)
	}

	// Links in note1 rewritten where supported; embed + block ref reported.
	got, _ := v.Read("daily/note1.md")
	want := "full [[archive/new]], bare [[new]], embed ![[old]], block [[old#^b1]]\n"
	if string(got) != want {
		t.Errorf("note1 = %q\nwant %q", got, want)
	}
	if len(report.Rewrites) != 2 {
		t.Errorf("report.Rewrites = %d, want 2", len(report.Rewrites))
	}
	if len(report.Issues) != 2 {
		t.Errorf("report.Issues = %d, want 2", len(report.Issues))
	}

	// Unrelated note untouched.
	if got, _ := v.Read("areas/other.md"); string(got) != "unrelated [[areas/health]]\n" {
		t.Errorf("unrelated note changed: %q", got)
	}
}

func TestFSDirectRemove(t *testing.T) {
	t.Parallel()
	v := newTestVault(t)
	mustWrite(t, v, "dir/a.md", "A")
	mustWrite(t, v, "dir/sub/b.md", "B")

	// Remove a single file.
	if err := v.Remove("dir/a.md"); err != nil {
		t.Fatalf("Remove file: %v", err)
	}
	if ok, _ := v.Exists("dir/a.md"); ok {
		t.Error("file should be gone")
	}
	// Remove a directory recursively.
	if err := v.Remove("dir"); err != nil {
		t.Fatalf("Remove dir: %v", err)
	}
	if ok, _ := v.Exists("dir/sub/b.md"); ok {
		t.Error("directory should be gone")
	}
	// Removing a non-existent path is not an error.
	if err := v.Remove("nonexistent"); err != nil {
		t.Errorf("removing a missing path should be a no-op, got %v", err)
	}
	// Removing outside the vault is refused.
	if err := v.Remove("../escape"); !errors.Is(err, ErrOutsideVault) {
		t.Errorf("Remove(../escape) = %v, want ErrOutsideVault", err)
	}
}

func TestFSDirectMoveRefusesClobber(t *testing.T) {
	t.Parallel()
	v := newTestVault(t)
	mustWrite(t, v, "a.md", "A")
	mustWrite(t, v, "b.md", "B")
	if _, err := v.Move("a.md", "b.md"); err == nil {
		t.Fatal("Move onto an existing file should fail")
	}
}

func mustWrite(t *testing.T, v Vault, rel, content string) {
	t.Helper()
	if err := v.Write(rel, []byte(content)); err != nil {
		t.Fatalf("Write %s: %v", rel, err)
	}
}
