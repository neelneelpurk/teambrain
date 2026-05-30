// Package sync implements promotion of notes from the personal brain to one or
// more team brains (1:n). A note routes itself by listing team names in its
// `teambrains:` frontmatter property; one note can target several teams.
// create-sync stages a normalized copy per team under _sync/<team>/, view-sync
// previews each team's payload with a diff and a link-integrity report, and
// commit-sync copies each team's payload into that team vault and commits
// exactly those paths. The link gate keeps each team brain free of dangling
// cross-vault links.
package sync

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

const (
	stageDir      = "_sync"
	teambrainsKey = "teambrains"     // frontmatter list: target team names
	destKey       = "teambrain_dest" // frontmatter scalar: optional dest override
	claudeDirName = ".claude"
)

// TeamTarget is one bound team vault the personal vault can promote to.
type TeamTarget struct {
	Name  string
	Vault vault.Vault
}

// Promoter coordinates promotion from a personal vault to its bound team vaults.
type Promoter struct {
	personal  vault.Vault
	teams     map[string]TeamTarget
	teamOrder []string // team names, sorted, for deterministic output
	git       git.Git
}

// NewPromoter constructs a Promoter over the given team targets.
func NewPromoter(personal vault.Vault, teams []TeamTarget, g git.Git) *Promoter {
	m := make(map[string]TeamTarget, len(teams))
	order := make([]string, 0, len(teams))
	for _, t := range teams {
		m[t.Name] = t
		order = append(order, t.Name)
	}
	sort.Strings(order)
	return &Promoter{personal: personal, teams: m, teamOrder: order, git: g}
}

// StagedItem describes one note staged for one team.
type StagedItem struct {
	Src       string `json:"src"`
	Team      string `json:"team"`
	Dest      string `json:"dest"`
	StagePath string `json:"stage_path"`
}

// CreateResult reports a create-sync.
type CreateResult struct {
	Staged   []StagedItem `json:"staged"`
	Warnings []string     `json:"warnings"`
}

// ViewItem is one note's preview against a team vault.
type ViewItem struct {
	Dest   string `json:"dest"`
	Status string `json:"status"` // new | modified | unchanged
	Diff   string `json:"diff,omitempty"`
}

// LinkIssue flags a wikilink that will not resolve in the team vault.
type LinkIssue struct {
	Note string `json:"note"`
	Link string `json:"link"`
}

// TeamView is the preview for one team.
type TeamView struct {
	Team       string      `json:"team"`
	Items      []ViewItem  `json:"items"`
	LinkIssues []LinkIssue `json:"link_issues"`
}

// ViewResult reports a view-sync across all teams with staged content.
type ViewResult struct {
	Teams []TeamView `json:"teams"`
}

// CommitOptions configures commit-sync.
type CommitOptions struct {
	Message string
	Push    bool
	DryRun  bool
	// Force promotes a team's payload even when it carries links that would
	// dangle in that team vault. Without it, such a payload is refused.
	Force bool
}

// TeamCommit reports a commit-sync for one team.
type TeamCommit struct {
	Team      string   `json:"team"`
	Committed []string `json:"committed"`
	Message   string   `json:"message"`
	Pushed    bool     `json:"pushed"`
}

// CommitResult reports a commit-sync across all teams.
type CommitResult struct {
	Teams []TeamCommit `json:"teams"`
}

// CreateSync stages notes for promotion. With explicit paths it stages those;
// with none it scans the whole personal vault for notes carrying a `teambrains:`
// property. Each note is staged once per target team under _sync/<team>/<dest>,
// with frontmatter normalized and the routing properties stripped from the
// promoted copy. Originals are untouched. References to unbound teams, and
// explicitly-named notes without a `teambrains:` property, are reported as
// warnings.
//
// A whole-vault scan recomputes the entire promotion set, so it first clears any
// prior staging — otherwise a note that was untagged or had its destination
// changed would leave an orphaned staged copy that commit-sync would still
// promote. Explicit paths are additive: they stage onto whatever is already
// there.
func (p *Promoter) CreateSync(paths []string, dryRun bool) (*CreateResult, error) {
	res := &CreateResult{Staged: []StagedItem{}, Warnings: []string{}}

	notes, explicit, err := p.selectNotes(paths)
	if err != nil {
		return nil, err
	}
	if !explicit && !dryRun {
		// Remove tolerates a missing path, so this is safe on a first run.
		if err := p.personal.Remove(stageDir); err != nil {
			return nil, err
		}
	}

	for _, src := range notes {
		content, err := p.personal.Read(src)
		if err != nil {
			return nil, exit.Userf("read %q: %v", src, err)
		}
		doc, err := vault.ParseDocument(content)
		if err != nil {
			return nil, exit.Userf("parse %q: %v", src, err)
		}

		targets := doc.GetList(teambrainsKey)
		if len(targets) == 0 {
			if explicit {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s has no %s property; not staged", src, teambrainsKey))
			}
			continue
		}

		dest := src
		if override, ok := doc.Get(destKey); ok && override != "" {
			dest = override
		}
		if err := validateDest(dest); err != nil {
			return nil, err
		}

		// The promoted copy carries no personal routing metadata.
		doc.Remove(teambrainsKey)
		doc.Remove(destKey)
		normalized, err := doc.Render()
		if err != nil {
			return nil, exit.Userf("normalize %q: %v", src, err)
		}

		for _, name := range dedupe(targets) {
			if _, ok := p.teams[name]; !ok {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s references unbound team %q; skipped", src, name))
				continue
			}
			stagePath := path.Join(stageDir, name, dest)
			if !dryRun {
				if err := p.personal.Write(stagePath, normalized); err != nil {
					return nil, err
				}
			}
			res.Staged = append(res.Staged, StagedItem{Src: src, Team: name, Dest: dest, StagePath: stagePath})
		}
	}
	return res, nil
}

// selectNotes returns the candidate note paths and whether they were given
// explicitly (vs. discovered by scanning).
func (p *Promoter) selectNotes(paths []string) (notes []string, explicit bool, err error) {
	if len(paths) > 0 {
		return paths, true, nil
	}
	all, err := p.personal.ListNotes(".")
	if err != nil {
		return nil, false, err
	}
	for _, n := range all {
		if strings.HasPrefix(n, stageDir+"/") || strings.HasPrefix(n, claudeDirName+"/") {
			continue
		}
		notes = append(notes, n)
	}
	return notes, false, nil
}

// ViewSync previews each team's staged payload: per-note status/diff against the
// team vault, plus a link-integrity report (a link resolves if its target is in
// the team vault or part of that team's payload).
func (p *Promoter) ViewSync() (*ViewResult, error) {
	res := &ViewResult{Teams: []TeamView{}}
	for _, name := range p.teamOrder {
		target := p.teams[name]
		staged, err := p.personal.ListNotes(path.Join(stageDir, name))
		if err != nil {
			return nil, err
		}
		if len(staged) == 0 {
			continue
		}

		tv := TeamView{Team: name, Items: []ViewItem{}, LinkIssues: []LinkIssue{}}
		prefix := path.Join(stageDir, name) + "/"
		for _, s := range staged {
			dest := strings.TrimPrefix(s, prefix)
			content, err := p.personal.Read(s)
			if err != nil {
				return nil, err
			}
			item := ViewItem{Dest: dest}
			existing, rerr := target.Vault.Read(dest)
			switch {
			case rerr != nil:
				item.Status = "new"
				item.Diff = lineDiff(nil, splitLines(content))
			case bytes.Equal(existing, content):
				item.Status = "unchanged"
			default:
				item.Status = "modified"
				item.Diff = lineDiff(splitLines(existing), splitLines(content))
			}
			tv.Items = append(tv.Items, item)
		}
		issues, err := p.teamLinkIssues(name)
		if err != nil {
			return nil, err
		}
		tv.LinkIssues = append(tv.LinkIssues, issues...)
		res.Teams = append(res.Teams, tv)
	}
	return res, nil
}

// CommitSync promotes each team's staged payload into that team vault and
// commits exactly those paths (tolerating an otherwise-dirty tree), optionally
// pushing, then clears the team's staging. Teams with no staged content are
// skipped.
func (p *Promoter) CommitSync(opts CommitOptions) (*CommitResult, error) {
	type plan struct {
		name  string
		dests []string
	}
	var plans []plan
	for _, name := range p.teamOrder {
		staged, err := p.personal.ListNotes(path.Join(stageDir, name))
		if err != nil {
			return nil, err
		}
		if len(staged) == 0 {
			continue
		}
		prefix := path.Join(stageDir, name) + "/"
		dests := make([]string, 0, len(staged))
		for _, s := range staged {
			dests = append(dests, strings.TrimPrefix(s, prefix))
		}
		sort.Strings(dests)
		plans = append(plans, plan{name: name, dests: dests})
	}
	if len(plans) == 0 {
		return nil, exit.Preconditionf("nothing staged").
			WithHint("stage notes with `teambrain create-sync` (or ask Claude Code to run the promote-to-team skill) first")
	}

	// Link-integrity gate: refuse a team's payload that would leave dangling
	// links in that team vault, unless the caller forces it. This is the
	// deterministic backstop behind the view-sync report.
	if !opts.Force {
		for _, pl := range plans {
			issues, err := p.teamLinkIssues(pl.name)
			if err != nil {
				return nil, err
			}
			if len(issues) > 0 {
				return nil, linkGateError(pl.name, issues)
			}
		}
	}

	// Pre-check: every team with content must be a git repo (fail before writing).
	if !opts.DryRun {
		for _, pl := range plans {
			root := p.teams[pl.name].Vault.Root()
			if !p.git.IsRepo(root) {
				return nil, exit.Externalf("team %q vault is not a git repository: %s", pl.name, root).
					WithHint("run `git init` in that vault (and add a remote to use --push)")
			}
		}
	}

	res := &CommitResult{Teams: []TeamCommit{}}
	for _, pl := range plans {
		target := p.teams[pl.name]
		for _, dest := range pl.dests {
			content, err := p.personal.Read(path.Join(stageDir, pl.name, dest))
			if err != nil {
				return res, err
			}
			if !opts.DryRun {
				if err := target.Vault.Write(dest, content); err != nil {
					return res, exit.Userf("write into team %q: %v", pl.name, err)
				}
			}
		}

		message := opts.Message
		if message == "" {
			message = defaultMessage(pl.name, pl.dests)
		}
		tc := TeamCommit{Team: pl.name, Committed: pl.dests, Message: message}
		if !opts.DryRun {
			root := target.Vault.Root()
			if err := p.git.Add(root, pl.dests); err != nil {
				return res, exit.Externalf("git add (%s): %v", pl.name, err)
			}
			if err := p.git.Commit(root, message, pl.dests); err != nil {
				return res, exit.Externalf("git commit (%s): %v", pl.name, err)
			}
			if opts.Push {
				if err := p.git.Push(root); err != nil {
					return res, exit.Externalf("git push (%s): %v", pl.name, err).
						WithHint("check the team vault's git remote and your push credentials")
				}
				tc.Pushed = true
			}
			if err := p.personal.Remove(path.Join(stageDir, pl.name)); err != nil {
				return res, err
			}
		}
		res.Teams = append(res.Teams, tc)
	}
	return res, nil
}

func knownNotes(v vault.Vault) (map[string]bool, error) {
	notes, err := v.ListNotes(".")
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, n := range notes {
		addKnown(known, n)
	}
	return known, nil
}

func addKnown(known map[string]bool, notePath string) {
	target := strings.TrimSuffix(notePath, ".md")
	known[target] = true
	known[path.Base(target)] = true
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func validateDest(dest string) error {
	clean := path.Clean(dest)
	if path.IsAbs(dest) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return exit.Userf("invalid promotion destination %q: must be a relative path inside the team vault", dest).
			WithHint("fix the note's `teambrain_dest:` frontmatter")
	}
	return nil
}

// teamLinkIssues returns the wikilinks in a team's staged payload that would not
// resolve in that team vault — a link resolves if its target already exists in
// the team vault or is part of the same staged payload. It is the single source
// of truth behind both the view-sync report and the commit-sync gate.
func (p *Promoter) teamLinkIssues(name string) ([]LinkIssue, error) {
	target, ok := p.teams[name]
	if !ok {
		return nil, nil
	}
	staged, err := p.personal.ListNotes(path.Join(stageDir, name))
	if err != nil {
		return nil, err
	}
	if len(staged) == 0 {
		return nil, nil
	}
	known, err := knownNotes(target.Vault)
	if err != nil {
		return nil, err
	}
	prefix := path.Join(stageDir, name) + "/"
	dests := make([]string, len(staged))
	for i, s := range staged {
		dests[i] = strings.TrimPrefix(s, prefix)
		addKnown(known, dests[i])
	}
	var issues []LinkIssue
	for i, s := range staged {
		content, err := p.personal.Read(s)
		if err != nil {
			return nil, err
		}
		for _, link := range vault.UnresolvedLinks(content, known) {
			issues = append(issues, LinkIssue{Note: dests[i], Link: link})
		}
	}
	return issues, nil
}

// linkGateError reports a refused promotion: links that would dangle in the team
// vault, with the override hint.
func linkGateError(team string, issues []LinkIssue) error {
	var b strings.Builder
	fmt.Fprintf(&b, "team %q has %d unresolved link(s) that would dangle after promotion:", team, len(issues))
	for _, li := range issues {
		fmt.Fprintf(&b, "\n  %s → [[%s]]", li.Note, li.Link)
	}
	return exit.Userf("%s", b.String()).
		WithHint("also-tag the targets, inline them, or fix the links — or pass --force to promote anyway")
}

func defaultMessage(team string, dests []string) string {
	noun := "notes"
	if len(dests) == 1 {
		noun = "note"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "teambrain: promote %d %s to %s\n\n", len(dests), noun, team)
	for _, d := range dests {
		fmt.Fprintf(&sb, "- %s\n", d)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func splitLines(b []byte) []string {
	s := string(b)
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// lineDiff returns a readable line diff via a longest-common-subsequence table:
// context lines prefixed "  ", removals "- ", additions "+ ". Deterministic.
func lineDiff(a, b []string) string {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var sb strings.Builder
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			fmt.Fprintf(&sb, "  %s\n", a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			fmt.Fprintf(&sb, "- %s\n", a[i])
			i++
		default:
			fmt.Fprintf(&sb, "+ %s\n", b[j])
			j++
		}
	}
	for ; i < n; i++ {
		fmt.Fprintf(&sb, "- %s\n", a[i])
	}
	for ; j < m; j++ {
		fmt.Fprintf(&sb, "+ %s\n", b[j])
	}
	return sb.String()
}
