// Package capability authors and inventories Claude Code capabilities — skills,
// agents, hooks, and commands — inside a .claude directory. Authoring (`new`) is
// deterministic file placement; hooks additionally merge into settings.json and
// record ownership in .teambrain.json. Inventory (`List`) is a live filesystem
// scan with no cache, so it can never desync from disk.
package capability

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/neelneelpurk/teambrain/internal/clock"
	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// Kind enumerates the capability types.
type Kind string

// The supported capability kinds.
const (
	KindSkill   Kind = "skill"
	KindAgent   Kind = "agent"
	KindHook    Kind = "hook"
	KindCommand Kind = "command"
)

// nameRe constrains capability names to a safe, path-free token.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// Store operates on a single .claude directory.
type Store struct {
	dir    string
	clk    clock.Clock
	dryRun bool
}

// OpenStore opens the store at claudeDir using the system clock.
func OpenStore(claudeDir string) *Store {
	return &Store{dir: claudeDir, clk: clock.System{}}
}

// OpenStoreWithClock is OpenStore with an injectable clock for deterministic
// timestamps in tests.
func OpenStoreWithClock(claudeDir string, clk clock.Clock) *Store {
	return &Store{dir: claudeDir, clk: clk}
}

// WithDryRun toggles dry-run mode (no writes) and returns the store for
// chaining.
func (s *Store) WithDryRun(v bool) *Store {
	s.dryRun = v
	return s
}

// Dir returns the .claude directory path.
func (s *Store) Dir() string { return s.dir }

// write honors dry-run mode: in dry-run it reports success without touching
// disk, so callers can still report the would-be changes.
func (s *Store) write(abs string, content []byte, mode os.FileMode) error {
	if s.dryRun {
		return nil
	}
	return writeFile(abs, content, mode)
}

// ListItem is one discovered capability.
type ListItem struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	Event       string `json:"event,omitempty"`
}

// NewResult reports the outcome of an authoring operation.
type NewResult struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Path    string   `json:"path"`
	Created []string `json:"created"`
	Changed bool     `json:"changed"`
}

func validateName(name string) error {
	if name == "" {
		return exit.Userf("capability name must not be empty")
	}
	if !nameRe.MatchString(name) {
		return exit.Userf("invalid capability name %q: use letters, digits, '.', '-', '_' (no path separators)", name)
	}
	return nil
}

// List scans the .claude directory and returns all capabilities found, sorted by
// kind then name. Hook events are annotated from the ownership manifest when
// available, but existence is always determined from disk.
func (s *Store) List() ([]ListItem, error) {
	var items []ListItem

	skills, err := s.listSkills()
	if err != nil {
		return nil, err
	}
	items = append(items, skills...)

	agents, err := s.listMarkdown("agents", KindAgent)
	if err != nil {
		return nil, err
	}
	items = append(items, agents...)

	commands, err := s.listMarkdown("commands", KindCommand)
	if err != nil {
		return nil, err
	}
	items = append(items, commands...)

	hooks, err := s.listHooks()
	if err != nil {
		return nil, err
	}
	items = append(items, hooks...)

	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Store) listSkills() ([]ListItem, error) {
	dir := filepath.Join(s.dir, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ignoreNotExist(err)
	}
	var items []ListItem
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("skills", e.Name(), "SKILL.md"))
		content, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(rel)))
		if err != nil {
			continue // a directory without SKILL.md is not a skill
		}
		items = append(items, ListItem{
			Name:        e.Name(),
			Kind:        string(KindSkill),
			Path:        rel,
			Description: description(content),
		})
	}
	return items, nil
}

func (s *Store) listMarkdown(sub string, kind Kind) ([]ListItem, error) {
	dir := filepath.Join(s.dir, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ignoreNotExist(err)
	}
	var items []ListItem
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(sub, e.Name()))
		content, _ := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(rel)))
		items = append(items, ListItem{
			Name:        strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
			Kind:        string(kind),
			Path:        rel,
			Description: description(content),
		})
	}
	return items, nil
}

func (s *Store) listHooks() ([]ListItem, error) {
	dir := filepath.Join(s.dir, "hooks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ignoreNotExist(err)
	}

	events := map[string]string{}
	if man, err := manifest.LoadClaude(s.dir); err == nil {
		for _, c := range man.Capabilities {
			if c.Type == string(KindHook) {
				events[c.Name] = c.Event
			}
		}
	}

	var items []ListItem
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		items = append(items, ListItem{
			Name:  name,
			Kind:  string(KindHook),
			Path:  filepath.ToSlash(filepath.Join("hooks", e.Name())),
			Event: events[name],
		})
	}
	return items, nil
}

// description extracts a note's frontmatter description, if any.
func description(content []byte) string {
	doc, err := vault.ParseDocument(content)
	if err != nil {
		return ""
	}
	d, _ := doc.Get("description")
	return d
}

func ignoreNotExist(err error) error {
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
