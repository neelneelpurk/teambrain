// Package scaffold creates and repairs vault directory trees: the content
// folders, the .claude capability folders, the seeded scaffolding skills, and
// the plain-JSON manifests. Every write is create-if-missing, so re-running init
// is a no-op when the vault is intact and a repair when a file has gone missing
// — and it never clobbers user edits or the team binding.
package scaffold

import (
	"sort"

	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/skills"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// Result reports what an init did: files newly created and files already
// present (skipped), both vault-relative and sorted.
type Result struct {
	Root     string   `json:"root"`
	Created  []string `json:"created"`
	Existing []string `json:"existing"`
}

// personalContentDirs are the PARA-style folders of a personal brain.
var personalContentDirs = []string{"inbox", "daily", "projects", "areas", "resources"}

// teamContentDirs are the shared folders of a team brain.
var teamContentDirs = []string{"adrs", "design-docs", "runbooks", "conventions", "mocs"}

// claudeDirs are the capability folders that start empty (so they get a .gitkeep).
var claudeDirs = []string{".claude/agents", ".claude/hooks", ".claude/commands"}

const claudeMDPersonal = `# Personal brain

A personal-brain vault managed with teambrain. It is a normal Obsidian vault and
a normal git repository; teambrain only adds the few file operations that git
and the filesystem cannot do safely on their own.

## Layout

- ` + "`inbox/`" + ` — capture first, sort later
- ` + "`daily/`" + ` — daily notes
- ` + "`projects/`" + ` — active, outcome-oriented work
- ` + "`areas/`" + ` — ongoing responsibilities
- ` + "`resources/`" + ` — reference material
- ` + "`.claude/`" + ` — skills, agents, hooks, and commands for Claude Code
- ` + "`_sync/`" + ` — staging for promotion to the team brain (gitignored)

## Promotion

First bind a team brain (once):

` + "```sh" + `
teambrain team bind <path|remote> --name <name>
` + "```" + `

Then tag a note for that team in its frontmatter — ` + "`teambrains: [<name>]`" + ` —
and ask Claude Code to **promote it to the team**; the seeded ` + "`promote-to-team`" + `
skill walks the flow. Under the hood it runs ` + "`teambrain create-sync` → `view-sync` → `commit-sync`" + `.
commit-sync refuses links that would dangle in the team vault unless you pass
` + "`--force`" + `, and confirms before writing to the shared repo.
`

const claudeMDTeam = `# Team brain

A shared team-brain vault managed with teambrain. It is a normal Obsidian vault
and a normal git repository. Notes are promoted here from teammates' personal
brains via ` + "`teambrain commit-sync`" + `, after a link-integrity check.

## Layout

- ` + "`adrs/`" + ` — architecture decision records
- ` + "`design-docs/`" + ` — design documents
- ` + "`runbooks/`" + ` — operational runbooks
- ` + "`conventions/`" + ` — shared conventions
- ` + "`mocs/`" + ` — maps of content
- ` + "`.claude/`" + ` — shared skills, agents, hooks, and commands
`

const gitignorePersonal = `# teambrain promotion staging (transient)
_sync/

# teambrain optional, rebuildable cache (never the source of truth)
.teambrain/

# OS / editor cruft
.DS_Store
.obsidian/workspace*.json
`

const gitignoreTeam = `# teambrain optional, rebuildable cache (never the source of truth)
.teambrain/

# OS / editor cruft
.DS_Store
.obsidian/workspace*.json
`

// PersonalVault scaffolds (or repairs) a personal-brain vault in v.
func PersonalVault(v vault.Vault, dryRun bool) (*Result, error) {
	return scaffoldVault(v, manifest.RolePersonal, personalContentDirs, claudeMDPersonal, gitignorePersonal, dryRun)
}

// TeamVault scaffolds (or repairs) a team-brain vault in v.
func TeamVault(v vault.Vault, dryRun bool) (*Result, error) {
	return scaffoldVault(v, manifest.RoleTeam, teamContentDirs, claudeMDTeam, gitignoreTeam, dryRun)
}

func scaffoldVault(v vault.Vault, role string, contentDirs []string, claudeMD, gitignore string, dryRun bool) (*Result, error) {
	res := &Result{Root: v.Root(), Created: []string{}, Existing: []string{}}

	rootManifest, err := manifest.Marshal(manifest.NewRoot(role))
	if err != nil {
		return nil, err
	}
	claudeManifest, err := manifest.Marshal(manifest.NewClaude())
	if err != nil {
		return nil, err
	}

	type file struct {
		rel     string
		content []byte
	}
	files := []file{
		{"CLAUDE.md", []byte(claudeMD)},
		{".gitignore", []byte(gitignore)},
		{manifest.FileName, rootManifest},
		{".claude/" + manifest.FileName, claudeManifest},
	}
	for _, d := range contentDirs {
		files = append(files, file{d + "/.gitkeep", nil})
	}
	for _, d := range claudeDirs {
		files = append(files, file{d + "/.gitkeep", nil})
	}

	seeds, err := skills.Seeds()
	if err != nil {
		return nil, err
	}
	for _, s := range seeds {
		files = append(files, file{s.RelPath, s.Content})
	}

	for _, f := range files {
		exists, err := v.Exists(f.rel)
		if err != nil {
			return nil, err
		}
		if exists {
			res.Existing = append(res.Existing, f.rel)
			continue
		}
		if !dryRun {
			if err := v.Write(f.rel, f.content); err != nil {
				return nil, err
			}
		}
		res.Created = append(res.Created, f.rel)
	}

	sort.Strings(res.Created)
	sort.Strings(res.Existing)
	return res, nil
}
