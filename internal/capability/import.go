package capability

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/manifest"
)

// Source is a candidate vault .claude directory to import from.
type Source struct {
	Label     string
	ClaudeDir string
}

// ImportOptions configures resolution of an import.
type ImportOptions struct {
	Name    string
	Kind    Kind
	Sources []Source
	From    string // narrows to the source with this Label
	Mode    string // "copy" (default) or "link"
}

// Plan is a resolved, not-yet-applied import. For hooks it carries the event,
// matcher, and a script preview so the CLI can show the script and confirm
// before any code lands.
type Plan struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	SourceLabel   string   `json:"source"`
	Mode          string   `json:"mode"`
	Files         []string `json:"files"`
	HookEvent     string   `json:"event,omitempty"`
	HookMatcher   string   `json:"matcher,omitempty"`
	ScriptPreview string   `json:"script_preview,omitempty"`

	sourceClaudeDir string
}

// ImportResult reports an applied import.
type ImportResult struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Source  string   `json:"source"`
	Mode    string   `json:"mode"`
	Files   []string `json:"files"`
	Changed bool     `json:"changed"`
}

// UninstallResult reports a removal.
type UninstallResult struct {
	Name    string   `json:"name"`
	Removed []string `json:"removed"`
	Missing []string `json:"missing,omitempty"`
	Changed bool     `json:"changed"`
}

// UpdateResult reports an in-place refresh from source.
type UpdateResult struct {
	Name        string `json:"name"`
	OldChecksum string `json:"old_checksum"`
	NewChecksum string `json:"new_checksum"`
	Changed     bool   `json:"changed"`
}

// primaryRel returns the capability's primary file relative to .claude.
func primaryRel(kind Kind, name string) string {
	switch kind {
	case KindSkill:
		return "skills/" + name + "/SKILL.md"
	case KindAgent:
		return "agents/" + name + ".md"
	case KindHook:
		return "hooks/" + name + ".sh"
	case KindCommand:
		return "commands/" + name + ".md"
	default:
		return ""
	}
}

// PlanImport resolves which source to import from and what files it entails,
// without writing anything. Ambiguity (the capability exists in more than one
// source and From is unset) is an error, never a silent pick.
func (s *Store) PlanImport(opts ImportOptions) (*Plan, error) {
	if err := validateName(opts.Name); err != nil {
		return nil, err
	}
	mode := opts.Mode
	if mode == "" {
		mode = "copy"
	}
	if mode != "copy" && mode != "link" {
		return nil, exit.Userf("invalid import mode %q: want copy or link", mode)
	}

	primary := primaryRel(opts.Kind, opts.Name)
	if primary == "" {
		return nil, exit.Userf("unknown capability kind %q", opts.Kind)
	}

	var candidates []Source
	for _, src := range opts.Sources {
		if opts.From != "" && src.Label != opts.From {
			continue
		}
		if fileExists(filepath.Join(src.ClaudeDir, filepath.FromSlash(primary))) {
			candidates = append(candidates, src)
		}
	}

	switch len(candidates) {
	case 0:
		if opts.From != "" {
			return nil, exit.Userf("%s %q not found in source %q", opts.Kind, opts.Name, opts.From)
		}
		return nil, exit.Userf("%s %q not found in any source", opts.Kind, opts.Name)
	case 1:
		// ok
	default:
		labels := make([]string, 0, len(candidates))
		for _, c := range candidates {
			labels = append(labels, c.Label)
		}
		sort.Strings(labels)
		return nil, exit.Userf("%s %q is ambiguous; found in %s", opts.Kind, opts.Name, strings.Join(labels, ", ")).
			WithHint("disambiguate with --from <" + strings.Join(labels, "|") + ">")
	}

	chosen := candidates[0]
	plan := &Plan{
		Name:            opts.Name,
		Kind:            string(opts.Kind),
		SourceLabel:     chosen.Label,
		Mode:            mode,
		sourceClaudeDir: chosen.ClaudeDir,
	}

	files, err := planFiles(opts.Kind, opts.Name, chosen.ClaudeDir, mode)
	if err != nil {
		return nil, err
	}
	plan.Files = files

	if opts.Kind == KindHook {
		if man, err := manifest.LoadClaude(chosen.ClaudeDir); err == nil {
			if entry, ok := man.Find(opts.Name); ok {
				plan.HookEvent = entry.Event
			}
		}
		if plan.HookEvent == "" {
			return nil, exit.Userf("hook %q in %q has no recorded event", opts.Name, chosen.Label).
				WithHint("recreate it with `teambrain hook new --event <Event>` in the source vault")
		}
		if data, err := os.ReadFile(filepath.Join(chosen.ClaudeDir, filepath.FromSlash(primary))); err == nil {
			plan.ScriptPreview = string(data)
		}
	}
	return plan, nil
}

// planFiles enumerates the target-relative files an import will write.
func planFiles(kind Kind, name, sourceClaude, mode string) ([]string, error) {
	if mode == "link" {
		// Link mode materializes a single symlink at the primary path (the skill
		// folder, or the agent/hook file).
		if kind == KindSkill {
			return []string{"skills/" + name}, nil
		}
		return []string{primaryRel(kind, name)}, nil
	}
	if kind != KindSkill {
		return []string{primaryRel(kind, name)}, nil
	}
	// Copy a skill: enumerate every file in the source skill folder.
	srcDir := filepath.Join(sourceClaude, "skills", name)
	var files []string
	err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(sourceClaude, p)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// Apply performs the import described by plan: copies or links the files, merges
// a hook's registration into settings.json, and records ownership with a
// checksum. It is idempotent; re-applying identical content reports Changed=false.
func (s *Store) Apply(plan *Plan) (*ImportResult, error) {
	res := &ImportResult{Name: plan.Name, Kind: plan.Kind, Source: plan.SourceLabel, Mode: plan.Mode, Files: plan.Files}

	var (
		changed bool
		err     error
	)
	if plan.Mode == "link" {
		changed, err = s.applyLink(plan)
	} else {
		changed, err = s.applyCopy(plan)
	}
	if err != nil {
		return nil, err
	}

	if Kind(plan.Kind) == KindHook {
		command := "$CLAUDE_PROJECT_DIR/.claude/hooks/" + plan.Name + ".sh"
		settingsAbs := filepath.Join(s.dir, "settings.json")
		raw, err := readIfExists(settingsAbs)
		if err != nil {
			return nil, err
		}
		merged, smChanged, err := MergeHook(raw, HookRegistration{Event: plan.HookEvent, Matcher: plan.HookMatcher, Command: command})
		if err != nil {
			return nil, exit.Userf("merge settings.json: %v", err)
		}
		if smChanged {
			if err := s.write(settingsAbs, merged, 0o644); err != nil {
				return nil, err
			}
			changed = true
		}
	}

	sum, err := s.primaryChecksum(plan)
	if err != nil {
		return nil, err
	}
	man, err := loadOrNewClaude(s.dir)
	if err != nil {
		return nil, err
	}
	installedAt := s.clk.Now().UTC().Format(time.RFC3339)
	if existing, ok := man.Find(plan.Name); ok {
		if existing.InstalledAt != "" {
			installedAt = existing.InstalledAt
		}
		if existing.Checksum != sum {
			changed = true
		}
	}
	man.Upsert(manifest.Capability{
		Name:        plan.Name,
		Type:        plan.Kind,
		Event:       plan.HookEvent,
		Source:      plan.SourceLabel,
		Checksum:    sum,
		Mode:        plan.Mode,
		Files:       plan.Files,
		InstalledAt: installedAt,
	})
	if !s.dryRun {
		if err := manifest.SaveClaude(s.dir, man); err != nil {
			return nil, err
		}
	}

	res.Changed = changed
	return res, nil
}

// applyCopy copies each planned file from the source, reporting whether any
// bytes changed.
func (s *Store) applyCopy(plan *Plan) (bool, error) {
	changed := false
	for _, rel := range plan.Files {
		srcAbs := filepath.Join(plan.sourceClaudeDir, filepath.FromSlash(rel))
		dstAbs := filepath.Join(s.dir, filepath.FromSlash(rel))
		data, err := os.ReadFile(srcAbs)
		if err != nil {
			return false, err
		}
		if existing, err := os.ReadFile(dstAbs); err == nil && bytes.Equal(existing, data) {
			continue
		}
		mode := os.FileMode(0o644)
		if Kind(plan.Kind) == KindHook {
			mode = 0o755
		}
		if err := s.write(dstAbs, data, mode); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

// applyLink materializes a symlink at the primary path pointing into the source.
func (s *Store) applyLink(plan *Plan) (bool, error) {
	rel := plan.Files[0]
	dstAbs := filepath.Join(s.dir, filepath.FromSlash(rel))
	var srcAbs string
	if Kind(plan.Kind) == KindSkill {
		srcAbs = filepath.Join(plan.sourceClaudeDir, "skills", plan.Name)
	} else {
		srcAbs = filepath.Join(plan.sourceClaudeDir, filepath.FromSlash(primaryRel(Kind(plan.Kind), plan.Name)))
	}
	if target, err := os.Readlink(dstAbs); err == nil && target == srcAbs {
		return false, nil
	}
	if s.dryRun {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return false, err
	}
	_ = os.Remove(dstAbs)
	if err := os.Symlink(srcAbs, dstAbs); err != nil {
		return false, exit.Externalf("create symlink: %v", err)
	}
	return true, nil
}

// primaryChecksum returns the checksum of the capability's primary file, read
// from the source for link mode (where the target is a symlink) or the target
// otherwise.
func (s *Store) primaryChecksum(plan *Plan) (string, error) {
	primary := primaryRel(Kind(plan.Kind), plan.Name)
	base := s.dir
	if plan.Mode == "link" {
		base = plan.sourceClaudeDir
	}
	data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(primary)))
	if err != nil {
		if s.dryRun {
			return "", nil
		}
		return "", err
	}
	return checksum(data), nil
}

// Uninstall removes a teambrain-owned capability: its files (or symlink), its
// settings.json registration (for hooks), and its manifest entry. It touches
// only owned items; foreign content is preserved. When the manifest becomes
// empty it is deleted, leaving no teambrain artifact behind.
func (s *Store) Uninstall(name string) (*UninstallResult, error) {
	man, err := manifest.LoadClaude(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, exit.Userf("nothing is installed here (no %s)", manifest.FileName)
		}
		return nil, err
	}
	entry, ok := man.Find(name)
	if !ok {
		return nil, exit.Userf("%q is not a teambrain-owned capability here", name).
			WithHint("run `teambrain <kind> list` to see what is installed")
	}

	res := &UninstallResult{Name: name}
	for _, rel := range entry.Files {
		abs := filepath.Join(s.dir, filepath.FromSlash(rel))
		if _, err := os.Lstat(abs); err != nil {
			res.Missing = append(res.Missing, rel)
			continue
		}
		if !s.dryRun {
			if err := os.RemoveAll(abs); err != nil {
				return nil, err
			}
			pruneEmptyParents(s.dir, filepath.Dir(abs))
		}
		res.Removed = append(res.Removed, rel)
		res.Changed = true
	}

	if entry.Type == string(KindHook) {
		settingsAbs := filepath.Join(s.dir, "settings.json")
		raw, err := readIfExists(settingsAbs)
		if err != nil {
			return nil, err
		}
		command := "$CLAUDE_PROJECT_DIR/.claude/hooks/" + name + ".sh"
		out, smChanged, err := UnmergeHook(raw, command)
		if err != nil {
			return nil, err
		}
		if smChanged && !s.dryRun {
			if err := s.write(settingsAbs, out, 0o644); err != nil {
				return nil, err
			}
			res.Changed = true
		}
	}

	man.Remove(name)
	if !s.dryRun {
		if len(man.Capabilities) == 0 {
			if err := os.Remove(filepath.Join(s.dir, manifest.FileName)); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		} else if err := manifest.SaveClaude(s.dir, man); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// Update re-imports an owned capability from its recorded source, refreshing the
// files and checksum. It reports whether the content drifted.
func (s *Store) Update(name string, sources []Source) (*UpdateResult, error) {
	man, err := manifest.LoadClaude(s.dir)
	if err != nil {
		return nil, exit.Userf("nothing is installed here: %v", err)
	}
	entry, ok := man.Find(name)
	if !ok {
		return nil, exit.Userf("%q is not installed here", name)
	}

	plan, err := s.PlanImport(ImportOptions{
		Name:    name,
		Kind:    Kind(entry.Type),
		Sources: sources,
		From:    entry.Source,
		Mode:    entry.Mode,
	})
	if err != nil {
		return nil, err
	}
	old := entry.Checksum
	if _, err := s.Apply(plan); err != nil {
		return nil, err
	}
	updated, _ := manifest.LoadClaude(s.dir)
	newEntry, _ := updated.Find(name)
	return &UpdateResult{
		Name:        name,
		OldChecksum: old,
		NewChecksum: newEntry.Checksum,
		Changed:     old != newEntry.Checksum,
	}, nil
}

// pruneEmptyParents removes now-empty directories from dir up to (but not
// including) root.
func pruneEmptyParents(root, dir string) {
	for {
		if dir == root || !strings.HasPrefix(dir, root) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
