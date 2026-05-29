package manifest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/testutil"
)

func TestRootRoundTripUnbound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRoot(RolePersonal)
	if err := SaveRoot(dir, r); err != nil {
		t.Fatalf("SaveRoot: %v", err)
	}
	got, err := LoadRoot(dir)
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if got.Vault != RolePersonal || got.Version != Version {
		t.Fatalf("loaded root = %+v", got)
	}
	if got.IsBound() {
		t.Fatal("a fresh root should be unbound")
	}

	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	testutil.AssertGolden(t, filepath.Join("testdata", "root_unbound.golden"), raw)
}

func TestRootRoundTripBound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRoot(RolePersonal)
	r.Team = &TeamBinding{
		Path:    "/home/u/team-brain",
		Remote:  "git@github.com:org/team-brain.git",
		BoundAt: "2026-05-29T12:00:00Z",
	}
	if err := SaveRoot(dir, r); err != nil {
		t.Fatalf("SaveRoot: %v", err)
	}
	got, err := LoadRoot(dir)
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if !got.IsBound() || got.Team.Path != "/home/u/team-brain" {
		t.Fatalf("binding not round-tripped: %+v", got.Team)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	testutil.AssertGolden(t, filepath.Join("testdata", "root_bound.golden"), raw)
}

func TestLoadRootMissingIsNotExist(t *testing.T) {
	t.Parallel()
	_, err := LoadRoot(t.TempDir())
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("LoadRoot on missing file = %v, want fs.ErrNotExist", err)
	}
}

func TestClaudeOwnershipRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	c := NewClaude()
	if c.Capabilities == nil {
		t.Fatal("Capabilities should be a non-nil empty slice")
	}
	c.Upsert(Capability{
		Name:        "format-on-save",
		Type:        "hook",
		Event:       "PostToolUse",
		Source:      "personal-brain",
		Checksum:    "sha256:abc123",
		Mode:        "copy",
		Files:       []string{"hooks/format-on-save.sh"},
		InstalledAt: "2026-05-29T12:00:00Z",
	})

	if err := SaveClaude(dir, c); err != nil {
		t.Fatalf("SaveClaude: %v", err)
	}
	got, err := LoadClaude(dir)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	entry, ok := got.Find("format-on-save")
	if !ok || entry.Type != "hook" || entry.Event != "PostToolUse" {
		t.Fatalf("capability not round-tripped: %+v", got.Capabilities)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	testutil.AssertGolden(t, filepath.Join("testdata", "claude_one_hook.golden"), raw)
}

func TestClaudeUpsertReplacesAndRemove(t *testing.T) {
	t.Parallel()
	c := NewClaude()
	c.Upsert(Capability{Name: "x", Type: "skill"})
	c.Upsert(Capability{Name: "x", Type: "skill", Checksum: "sha256:new"})
	if len(c.Capabilities) != 1 {
		t.Fatalf("Upsert should replace, got %d entries", len(c.Capabilities))
	}
	if got, _ := c.Find("x"); got.Checksum != "sha256:new" {
		t.Fatalf("Upsert did not replace checksum: %+v", got)
	}
	if !c.Remove("x") {
		t.Fatal("Remove should report true for an existing capability")
	}
	if c.Remove("x") {
		t.Fatal("Remove should report false for a missing capability")
	}
	if len(c.Capabilities) != 0 {
		t.Fatalf("after Remove, expected 0 entries, got %d", len(c.Capabilities))
	}
}
