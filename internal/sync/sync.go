// Package sync implements promotion of notes from the personal brain to the
// team brain: create-sync stages a copy into _sync/, view-sync previews the
// payload with a diff and a link-integrity report, and commit-sync copies the
// payload into the team vault and commits exactly those paths. The link gate is
// the one check that keeps the team brain free of dangling cross-vault links.
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

// stageDir is the staging folder inside the personal vault (gitignored).
const stageDir = "_sync"

// Promoter coordinates promotion between a personal vault and its bound team
// vault, using git for the commit. team may be nil until a team is bound;
// view-sync and commit-sync require it.
type Promoter struct {
	personal vault.Vault
	team     vault.Vault
	git      git.Git
}

// NewPromoter constructs a Promoter.
func NewPromoter(personal, team vault.Vault, g git.Git) *Promoter {
	return &Promoter{personal: personal, team: team, git: g}
}

// Spec names a note to promote and its destination in the team vault. An empty
// Dest defaults to Src.
type Spec struct {
	Src  string
	Dest string
}

// StagedItem describes one staged note.
type StagedItem struct {
	Src       string `json:"src"`
	Dest      string `json:"dest"`
	StagePath string `json:"stage_path"`
}

// CreateResult reports a create-sync.
type CreateResult struct {
	Staged []StagedItem `json:"staged"`
}

// ViewItem is one note's preview against the team vault.
type ViewItem struct {
	Dest   string `json:"dest"`
	Status string `json:"status"` // new | modified | unchanged
	Diff   string `json:"diff,omitempty"`
}

// LinkIssue flags a wikilink in a staged note that will not resolve in the team
// vault even after the whole payload is promoted.
type LinkIssue struct {
	Note string `json:"note"`
	Link string `json:"link"`
}

// ViewResult reports a view-sync.
type ViewResult struct {
	Items      []ViewItem  `json:"items"`
	LinkIssues []LinkIssue `json:"link_issues"`
}

// CommitOptions configures commit-sync.
type CommitOptions struct {
	Message string
	Push    bool
	DryRun  bool
}

// CommitResult reports a commit-sync.
type CommitResult struct {
	Committed []string `json:"committed"`
	Message   string   `json:"message"`
	Pushed    bool     `json:"pushed"`
}

// CreateSync stages each spec into the personal vault's _sync/ folder, mirroring
// the team destination, with frontmatter normalized. Originals are untouched.
func (p *Promoter) CreateSync(specs []Spec, dryRun bool) (*CreateResult, error) {
	res := &CreateResult{Staged: []StagedItem{}}
	for _, spec := range specs {
		dest := spec.Dest
		if dest == "" {
			dest = spec.Src
		}
		if err := validateDest(dest); err != nil {
			return nil, err
		}
		content, err := p.personal.Read(spec.Src)
		if err != nil {
			return nil, exit.Userf("read %q: %v", spec.Src, err)
		}
		normalized, err := normalize(content)
		if err != nil {
			return nil, exit.Userf("normalize %q: %v", spec.Src, err)
		}
		stagePath := path.Join(stageDir, dest)
		if !dryRun {
			if err := p.personal.Write(stagePath, normalized); err != nil {
				return nil, err
			}
		}
		res.Staged = append(res.Staged, StagedItem{Src: spec.Src, Dest: dest, StagePath: stagePath})
	}
	return res, nil
}

// ViewSync previews the staged payload: per-note status/diff against the team
// vault, and a link-integrity report. A link resolves if its target exists in
// the team vault or is itself part of this promotion payload.
func (p *Promoter) ViewSync() (*ViewResult, error) {
	if p.team == nil {
		return nil, errNoTeam()
	}
	staged, err := p.personal.ListNotes(stageDir)
	if err != nil {
		return nil, err
	}

	known, err := p.knownTeamNotes()
	if err != nil {
		return nil, err
	}
	dests := make([]string, len(staged))
	for i, s := range staged {
		dests[i] = strings.TrimPrefix(s, stageDir+"/")
		addKnown(known, dests[i])
	}

	res := &ViewResult{Items: []ViewItem{}, LinkIssues: []LinkIssue{}}
	for i, s := range staged {
		dest := dests[i]
		content, err := p.personal.Read(s)
		if err != nil {
			return nil, err
		}

		item := ViewItem{Dest: dest}
		existing, rerr := p.team.Read(dest)
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
		res.Items = append(res.Items, item)

		for _, link := range vault.UnresolvedLinks(content, known) {
			res.LinkIssues = append(res.LinkIssues, LinkIssue{Note: dest, Link: link})
		}
	}
	return res, nil
}

// CommitSync copies the staged payload into the team vault, stages and commits
// exactly those paths (tolerating an otherwise-dirty tree), optionally pushes,
// and clears _sync/.
func (p *Promoter) CommitSync(opts CommitOptions) (*CommitResult, error) {
	if p.team == nil {
		return nil, errNoTeam()
	}
	staged, err := p.personal.ListNotes(stageDir)
	if err != nil {
		return nil, err
	}
	if len(staged) == 0 {
		return nil, exit.Preconditionf("nothing staged").WithHint("run `teambrain create-sync <paths>` first")
	}
	if !opts.DryRun && !p.git.IsRepo(p.team.Root()) {
		return nil, exit.Externalf("team vault is not a git repository: %s", p.team.Root())
	}

	dests := make([]string, 0, len(staged))
	for _, s := range staged {
		dest := strings.TrimPrefix(s, stageDir+"/")
		content, err := p.personal.Read(s)
		if err != nil {
			return nil, err
		}
		if !opts.DryRun {
			if err := p.team.Write(dest, content); err != nil {
				return nil, exit.Userf("write into team vault: %v", err)
			}
		}
		dests = append(dests, dest)
	}
	sort.Strings(dests)

	message := opts.Message
	if message == "" {
		message = defaultMessage(dests)
	}
	if opts.DryRun {
		return &CommitResult{Committed: dests, Message: message, Pushed: false}, nil
	}

	if err := p.git.Add(p.team.Root(), dests); err != nil {
		return nil, exit.Externalf("git add: %v", err)
	}
	if err := p.git.Commit(p.team.Root(), message, dests); err != nil {
		return nil, exit.Externalf("git commit: %v", err)
	}
	pushed := false
	if opts.Push {
		if err := p.git.Push(p.team.Root()); err != nil {
			return nil, exit.Externalf("git push: %v", err)
		}
		pushed = true
	}
	if err := p.personal.Remove(stageDir); err != nil {
		return nil, err
	}
	return &CommitResult{Committed: dests, Message: message, Pushed: pushed}, nil
}

func (p *Promoter) knownTeamNotes() (map[string]bool, error) {
	notes, err := p.team.ListNotes(".")
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

func normalize(content []byte) ([]byte, error) {
	doc, err := vault.ParseDocument(content)
	if err != nil {
		return nil, err
	}
	return doc.Render()
}

func validateDest(dest string) error {
	clean := path.Clean(dest)
	if path.IsAbs(dest) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return exit.Userf("invalid promotion destination %q", dest)
	}
	return nil
}

func defaultMessage(dests []string) string {
	noun := "notes"
	if len(dests) == 1 {
		noun = "note"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "teambrain: promote %d %s\n\n", len(dests), noun)
	for _, d := range dests {
		fmt.Fprintf(&sb, "- %s\n", d)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func errNoTeam() error {
	return exit.Preconditionf("no team vault bound").
		WithHint("bind one with `teambrain team bind <path|remote>`")
}

func splitLines(b []byte) []string {
	s := string(b)
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// lineDiff returns a readable line diff: context lines are prefixed "  ",
// removals "- ", and additions "+ ". It is computed via a longest-common-
// subsequence table so output is deterministic (golden-friendly).
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
