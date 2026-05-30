package scaffold

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

func deleteFile(t *testing.T, v *vault.FSDirect, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(v.Root(), filepath.FromSlash(rel))); err != nil {
		t.Fatalf("delete %s: %v", rel, err)
	}
}

func newVault(t *testing.T) *vault.FSDirect {
	t.Helper()
	v, err := vault.NewFSDirect(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSDirect: %v", err)
	}
	return v
}

func TestPersonalVaultCreatesTree(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	res, err := PersonalVault(v, false)
	if err != nil {
		t.Fatalf("PersonalVault: %v", err)
	}
	if len(res.Created) == 0 || len(res.Existing) != 0 {
		t.Fatalf("first run should create everything: created=%d existing=%d", len(res.Created), len(res.Existing))
	}

	mustExist := []string{
		"CLAUDE.md",
		".gitignore",
		".teambrain.json",
		".claude/.teambrain.json",
		".claude/skills/create-teambrain-skill/SKILL.md",
		".claude/skills/create-teambrain-agent/SKILL.md",
		".claude/skills/create-teambrain-hook/SKILL.md",
		".claude/agents/.gitkeep",
		".claude/hooks/.gitkeep",
		".claude/commands/.gitkeep",
		"inbox/.gitkeep",
		"daily/.gitkeep",
		"projects/.gitkeep",
		"areas/.gitkeep",
		"resources/.gitkeep",
	}
	for _, p := range mustExist {
		if ok, _ := v.Exists(p); !ok {
			t.Errorf("expected %q to exist after init", p)
		}
	}

	// Root manifest is a valid, unbound personal vault.
	root, err := manifest.LoadRoot(v.Root())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if root.Vault != manifest.RolePersonal || root.IsBound() {
		t.Fatalf("unexpected root manifest: %+v", root)
	}

	// Seeded skills carry valid frontmatter.
	raw, _ := v.Read(".claude/skills/create-teambrain-skill/SKILL.md")
	doc, err := vault.ParseDocument(raw)
	if err != nil || !doc.HasFrontmatter {
		t.Fatalf("seeded skill invalid: %v", err)
	}
	if name, _ := doc.Get("name"); name != "create-teambrain-skill" {
		t.Fatalf("seeded skill name = %q", name)
	}
}

func TestTeamVaultCreatesTeamTree(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	if _, err := TeamVault(v, false); err != nil {
		t.Fatalf("TeamVault: %v", err)
	}
	for _, p := range []string{"CLAUDE.md", "adrs/.gitkeep", "runbooks/.gitkeep", "conventions/.gitkeep", "mocs/.gitkeep"} {
		if ok, _ := v.Exists(p); !ok {
			t.Errorf("expected %q in team vault", p)
		}
	}
	// Team vaults have no _sync staging folder.
	if ok, _ := v.Exists("_sync"); ok {
		t.Error("team vault should not have a _sync folder")
	}
	root, err := manifest.LoadRoot(v.Root())
	if err != nil {
		t.Fatal(err)
	}
	if root.Vault != manifest.RoleTeam {
		t.Fatalf("team manifest role = %q, want team", root.Vault)
	}
}

// TestGeneratedClaudeMDEncodesRetrievalContract pins the ambient instruction
// that shapes every Claude Code session in a brain: retrieve from the vault via
// the search-brain skill before answering, rather than guessing. This is the
// product's central "use the LLM correctly" rule, so it must live in the
// generated CLAUDE.md and not only in a skill that fires on a trigger.
func TestGeneratedClaudeMDEncodesRetrievalContract(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		scaffold func(vault.Vault, bool) (*Result, error)
	}{
		{"personal", PersonalVault},
		{"team", TeamVault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := newVault(t)
			if _, err := tc.scaffold(v, false); err != nil {
				t.Fatalf("scaffold: %v", err)
			}
			raw, err := v.Read("CLAUDE.md")
			if err != nil {
				t.Fatalf("read CLAUDE.md: %v", err)
			}
			body := strings.ToLower(string(raw))
			for _, want := range []string{"search-brain", "retrieve"} {
				if !strings.Contains(body, want) {
					t.Errorf("%s CLAUDE.md should encode the retrieval contract (missing %q)", tc.name, want)
				}
			}
		})
	}
}

func TestPersonalVaultIdempotentAndRepairs(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	first, err := PersonalVault(v, false)
	if err != nil {
		t.Fatalf("first PersonalVault: %v", err)
	}

	// Second run is a no-op: nothing created, everything already present.
	second, err := PersonalVault(v, false)
	if err != nil {
		t.Fatalf("second PersonalVault: %v", err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("re-init created files (not a no-op): %v", second.Created)
	}
	if len(second.Existing) != len(first.Created) {
		t.Fatalf("re-init existing=%d, want %d", len(second.Existing), len(first.Created))
	}

	// Re-init preserves a user-set team binding (must not clobber the manifest).
	root, _ := manifest.LoadRoot(v.Root())
	root.UpsertTeam(manifest.TeamBinding{Name: "team", Path: "/somewhere/team"})
	if err := manifest.SaveRoot(v.Root(), root); err != nil {
		t.Fatal(err)
	}

	// Delete a seed; re-init should repair exactly that file.
	const seed = ".claude/skills/create-teambrain-hook/SKILL.md"
	deleteFile(t, v, seed)

	third, err := PersonalVault(v, false)
	if err != nil {
		t.Fatalf("third PersonalVault: %v", err)
	}
	if !slices.Equal(third.Created, []string{seed}) {
		t.Fatalf("repair created = %v, want exactly [%s]", third.Created, seed)
	}
	if ok, _ := v.Exists(seed); !ok {
		t.Fatal("deleted seed should have been repaired")
	}

	// Binding survived the repair.
	root, _ = manifest.LoadRoot(v.Root())
	if b, ok := root.Team("team"); !ok || b.Path != "/somewhere/team" {
		t.Fatalf("re-init clobbered the team binding: %+v", root.Teams)
	}
}
