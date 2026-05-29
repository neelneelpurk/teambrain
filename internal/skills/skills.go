// Package skills embeds the Claude Code skills that ship inside the teambrain
// binary. There are two groups under assets/:
//
//   - scaffold/ — the create-teambrain-* skills that bootstrap authoring;
//   - library/  — a curated set of high-signal engineering skills.
//
// Both are seeded into a new vault's .claude/skills on init, so a fresh brain is
// immediately useful with Claude Code and no other LLM API. The library is also
// a discoverable catalog that can be installed into any .claude on demand.
package skills

import (
	"embed"
	"io/fs"
	"path"
	"sort"

	"github.com/neelneelpurk/teambrain/internal/vault"
)

//go:embed all:assets
var assets embed.FS

// Seed is one file to write into a vault, relative to the vault root.
type Seed struct {
	RelPath string
	Content []byte
}

// Seeds returns every embedded skill (scaffolders and library), each mapped to
// its .claude/skills/<name>/SKILL.md destination, sorted by path. Folder names
// are unique across groups, so both collapse into one skills directory.
func Seeds() ([]Seed, error) {
	var seeds []Seed
	err := fs.WalkDir(assets, "assets", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path.Base(p) != "SKILL.md" {
			return nil
		}
		content, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		name := path.Base(path.Dir(p))
		seeds = append(seeds, Seed{RelPath: ".claude/skills/" + name + "/SKILL.md", Content: content})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].RelPath < seeds[j].RelPath })
	return seeds, nil
}

// CatalogEntry is one installable library skill.
type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     []byte `json:"-"`
}

// Library returns the curated, installable work skills (everything under
// assets/library), sorted by name. The scaffolders are not part of the catalog.
func Library() ([]CatalogEntry, error) {
	var entries []CatalogEntry
	err := fs.WalkDir(assets, "assets/library", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path.Base(p) != "SKILL.md" {
			return nil
		}
		content, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		desc := ""
		if doc, err := vault.ParseDocument(content); err == nil {
			desc, _ = doc.Get("description")
		}
		entries = append(entries, CatalogEntry{
			Name:        path.Base(path.Dir(p)),
			Description: desc,
			Content:     content,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// LibrarySkill returns a single library skill by name.
func LibrarySkill(name string) (CatalogEntry, bool) {
	lib, err := Library()
	if err != nil {
		return CatalogEntry{}, false
	}
	for _, e := range lib {
		if e.Name == name {
			return e, true
		}
	}
	return CatalogEntry{}, false
}
