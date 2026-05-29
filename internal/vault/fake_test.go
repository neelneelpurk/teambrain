package vault

import (
	"errors"
	"testing"
)

// compile-time proof both backends satisfy the interface.
var (
	_ Vault = (*FSDirect)(nil)
	_ Vault = (*FakeVault)(nil)
)

func TestFakeVaultRoundTripAndContainment(t *testing.T) {
	t.Parallel()
	f := NewFakeVault()

	if err := f.Write("a/b.md", []byte("hi")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if ok, _ := f.Exists("a/b.md"); !ok {
		t.Fatal("Exists should be true")
	}
	got, _ := f.Read("a/b.md")
	if string(got) != "hi" {
		t.Fatalf("Read = %q", got)
	}

	for _, bad := range []string{"../x.md", "/abs.md", ".."} {
		if err := f.Write(bad, []byte("x")); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("Write(%q) = %v, want ErrOutsideVault", bad, err)
		}
	}
}

// TestFakeVaultMoveParity confirms the fake reproduces FSDirect's move/rewrite
// semantics, so tests that depend on the fake stay faithful.
func TestFakeVaultMoveParity(t *testing.T) {
	t.Parallel()
	f := NewFakeVault()
	_ = f.Write("projects/old.md", []byte("# Old\n"))
	_ = f.Write("daily/n.md", []byte("full [[projects/old]], bare [[old]], embed ![[old]]\n"))

	report, err := f.Move("projects/old.md", "archive/new.md")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	got, _ := f.Read("daily/n.md")
	want := "full [[archive/new]], bare [[new]], embed ![[old]]\n"
	if string(got) != want {
		t.Fatalf("rewritten = %q, want %q", got, want)
	}
	if len(report.Rewrites) != 2 || len(report.Issues) != 1 {
		t.Fatalf("report rewrites=%d issues=%d, want 2 and 1", len(report.Rewrites), len(report.Issues))
	}
	if ok, _ := f.Exists("projects/old.md"); ok {
		t.Error("source should be gone")
	}
}

func TestFakeVaultRemove(t *testing.T) {
	t.Parallel()
	f := NewFakeVault()
	_ = f.Write("dir/a.md", []byte("A"))
	_ = f.Write("dir/sub/b.md", []byte("B"))
	_ = f.Write("other.md", []byte("O"))

	// Removing a directory prefix removes everything under it.
	if err := f.Remove("dir"); err != nil {
		t.Fatalf("Remove dir: %v", err)
	}
	if ok, _ := f.Exists("dir/a.md"); ok {
		t.Error("dir/a.md should be gone")
	}
	if ok, _ := f.Exists("dir/sub/b.md"); ok {
		t.Error("dir/sub/b.md should be gone")
	}
	if ok, _ := f.Exists("other.md"); !ok {
		t.Error("unrelated file must survive")
	}
}
