package skills

import (
	"path"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/vault"
)

func TestSeedsAreValidAndIncludeScaffolders(t *testing.T) {
	t.Parallel()

	seeds, err := Seeds()
	if err != nil {
		t.Fatalf("Seeds: %v", err)
	}
	if len(seeds) < 10 {
		t.Fatalf("expected the scaffolders plus the library (>=10 seeds), got %d", len(seeds))
	}

	wantScaffolders := map[string]bool{
		"create-teambrain-skill": false,
		"create-teambrain-agent": false,
		"create-teambrain-hook":  false,
	}

	for _, s := range seeds {
		if !strings.HasPrefix(s.RelPath, ".claude/skills/") || !strings.HasSuffix(s.RelPath, "/SKILL.md") {
			t.Errorf("unexpected seed path %q", s.RelPath)
			continue
		}
		folder := path.Base(path.Dir(s.RelPath))

		doc, err := vault.ParseDocument(s.Content)
		if err != nil || !doc.HasFrontmatter {
			t.Errorf("seed %s: invalid frontmatter: %v", s.RelPath, err)
			continue
		}
		if name, _ := doc.Get("name"); name != folder {
			t.Errorf("seed %s: frontmatter name=%q, want %q", s.RelPath, name, folder)
		}
		if desc, _ := doc.Get("description"); strings.TrimSpace(desc) == "" {
			t.Errorf("seed %s: missing description", s.RelPath)
		}
		if _, ok := wantScaffolders[folder]; ok {
			wantScaffolders[folder] = true
		}
	}

	for name, seen := range wantScaffolders {
		if !seen {
			t.Errorf("scaffolder %q missing from seeds", name)
		}
	}
}

func TestLibraryIsCuratedAndExcludesScaffolders(t *testing.T) {
	t.Parallel()

	lib, err := Library()
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if len(lib) < 5 {
		t.Fatalf("expected a curated library, got %d entries", len(lib))
	}

	for _, e := range lib {
		if strings.HasPrefix(e.Name, "create-teambrain-") {
			t.Errorf("scaffolder %q must not appear in the library catalog", e.Name)
		}
		if e.Description == "" {
			t.Errorf("library skill %q has no description", e.Name)
		}
		if len(e.Content) == 0 {
			t.Errorf("library skill %q has no content", e.Name)
		}
	}

	// A few of the high-signal skills we expect to ship, including the
	// Obsidian-powered retrieval skill.
	for _, must := range []string{"code-review", "write-tests", "debug", "search-brain"} {
		if _, ok := LibrarySkill(must); !ok {
			t.Errorf("expected library skill %q", must)
		}
	}
	if _, ok := LibrarySkill("nope"); ok {
		t.Error("LibrarySkill should report missing skills as not found")
	}
}

// TestLibrarySkillsDriveObsidianCLI locks in the product requirement that every
// embedded library skill leans on the Obsidian CLI to find relevant files and
// get things done — not generic guidance.
func TestLibrarySkillsDriveObsidianCLI(t *testing.T) {
	t.Parallel()

	lib, err := Library()
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	for _, e := range lib {
		body := strings.ToLower(string(e.Content))
		if !strings.Contains(body, "obsidian") {
			t.Errorf("library skill %q does not reference Obsidian", e.Name)
		}
		if !strings.Contains(body, "obsidian --help") && !strings.Contains(body, "obsidian cli") {
			t.Errorf("library skill %q should point at the Obsidian CLI concretely", e.Name)
		}
	}
}
