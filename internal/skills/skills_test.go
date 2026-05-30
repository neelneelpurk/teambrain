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
		"promote-to-team":        false,
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

// TestLibrarySkillsRouteRetrievalThroughSearchBrain locks in the retrieval
// contract: search-brain is the single source of truth for *how* to read the
// vault (Obsidian MCP preferred, the CLI as a fallback). Every other library
// skill that needs vault context must delegate to it by name instead of
// hardcoding `obsidian --help` — that CLI-first invocation inverts the
// documented MCP-first priority and pins the prompt to a brittle CLI contract.
func TestLibrarySkillsRouteRetrievalThroughSearchBrain(t *testing.T) {
	t.Parallel()

	lib, err := Library()
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	for _, e := range lib {
		body := strings.ToLower(string(e.Content))

		if e.Name == "search-brain" {
			// search-brain *is* the contract, so it alone names the concrete
			// retrieval tools — both paths, MCP before the CLI.
			if !strings.Contains(body, "obsidian mcp") || !strings.Contains(body, "obsidian cli") {
				t.Errorf("search-brain must name both the Obsidian MCP and CLI retrieval paths")
			}
			continue
		}

		// Every work skill delegates retrieval to search-brain by name...
		if !strings.Contains(body, "search-brain") {
			t.Errorf("library skill %q must route retrieval through the search-brain skill", e.Name)
		}
		// ...and must not re-pin itself to the CLI-first invocation that
		// inverts the MCP-preferred contract.
		if strings.Contains(body, "obsidian --help") {
			t.Errorf("library skill %q hardcodes `obsidian --help`; delegate to search-brain instead", e.Name)
		}
	}
}
