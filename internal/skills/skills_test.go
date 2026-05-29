package skills

import (
	"path"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/vault"
)

func TestSeedsAreThreeValidScaffolders(t *testing.T) {
	t.Parallel()

	seeds, err := Seeds()
	if err != nil {
		t.Fatalf("Seeds: %v", err)
	}

	wantNames := map[string]bool{
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
		if err != nil {
			t.Errorf("seed %s: parse frontmatter: %v", s.RelPath, err)
			continue
		}
		if !doc.HasFrontmatter {
			t.Errorf("seed %s has no frontmatter", s.RelPath)
		}
		name, ok := doc.Get("name")
		if !ok || name != folder {
			t.Errorf("seed %s: frontmatter name=%q, want folder name %q", s.RelPath, name, folder)
		}
		if desc, ok := doc.Get("description"); !ok || strings.TrimSpace(desc) == "" {
			t.Errorf("seed %s: missing/empty description", s.RelPath)
		}
		if _, known := wantNames[name]; known {
			wantNames[name] = true
		} else {
			t.Errorf("unexpected seed name %q", name)
		}
	}

	for name, seen := range wantNames {
		if !seen {
			t.Errorf("expected seed %q was not produced", name)
		}
	}
}
