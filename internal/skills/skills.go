// Package skills embeds the Claude Code skills that teambrain seeds into a new
// vault's .claude/ directory on `init`. They are plain SKILL.md files compiled
// into the binary via go:embed, so a fresh vault is immediately usable and the
// tool carries no external asset dependency.
package skills

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:assets
var assets embed.FS

// Seed is one file to write into a vault, relative to the vault root.
type Seed struct {
	// RelPath is the destination relative to the vault root, e.g.
	// ".claude/skills/create-teambrain-skill/SKILL.md".
	RelPath string
	// Content is the file's bytes.
	Content []byte
}

// Seeds returns every embedded seed file, mapped from the internal assets/
// layout to its .claude/ destination, sorted by path for deterministic output.
func Seeds() ([]Seed, error) {
	var seeds []Seed
	err := fs.WalkDir(assets, "assets", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		content, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		seeds = append(seeds, Seed{
			RelPath: ".claude/" + strings.TrimPrefix(p, "assets/"),
			Content: content,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].RelPath < seeds[j].RelPath })
	return seeds, nil
}
