package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/manifest"
)

// HookOptions configures NewHook.
type HookOptions struct {
	Name        string
	Event       string
	Matcher     string
	Description string
}

// NewSkill creates .claude/skills/<name>/SKILL.md with valid frontmatter and a
// stub body. It errors if the skill already exists.
func (s *Store) NewSkill(name, description string) (*NewResult, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(filepath.Join("skills", name, "SKILL.md"))
	abs := filepath.Join(s.dir, filepath.FromSlash(rel))
	if fileExists(abs) {
		return nil, exit.Userf("skill %q already exists at %s", name, rel)
	}
	if err := s.write(abs, renderSkill(name, description), 0o644); err != nil {
		return nil, err
	}
	return &NewResult{Name: name, Kind: string(KindSkill), Path: rel, Created: []string{rel}, Changed: true}, nil
}

// NewAgent creates .claude/agents/<name>.md with valid frontmatter and a stub
// system prompt. It errors if the agent already exists.
func (s *Store) NewAgent(name, description string) (*NewResult, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(filepath.Join("agents", name+".md"))
	abs := filepath.Join(s.dir, filepath.FromSlash(rel))
	if fileExists(abs) {
		return nil, exit.Userf("agent %q already exists at %s", name, rel)
	}
	if err := s.write(abs, renderAgent(name, description), 0o644); err != nil {
		return nil, err
	}
	return &NewResult{Name: name, Kind: string(KindAgent), Path: rel, Created: []string{rel}, Changed: true}, nil
}

// NewHook writes .claude/hooks/<name>.sh, merges its registration into
// settings.json, and records ownership (with a checksum) in .teambrain.json. It
// is idempotent: re-running preserves an existing script and does not duplicate
// the settings entry.
func (s *Store) NewHook(opts HookOptions) (*NewResult, error) {
	if err := validateName(opts.Name); err != nil {
		return nil, err
	}
	if opts.Event == "" {
		return nil, exit.Userf("hook %q requires an event", opts.Name).
			WithHint("pass --event, e.g. --event PostToolUse")
	}

	scriptRel := filepath.ToSlash(filepath.Join("hooks", opts.Name+".sh"))
	scriptAbs := filepath.Join(s.dir, filepath.FromSlash(scriptRel))
	res := &NewResult{Name: opts.Name, Kind: string(KindHook), Path: scriptRel, Created: []string{}}

	if !fileExists(scriptAbs) {
		if err := s.write(scriptAbs, renderHook(opts.Name, opts.Event), 0o755); err != nil {
			return nil, err
		}
		res.Created = append(res.Created, scriptRel)
		res.Changed = true
	}

	command := "$CLAUDE_PROJECT_DIR/.claude/hooks/" + opts.Name + ".sh"
	settingsAbs := filepath.Join(s.dir, "settings.json")
	raw, err := readIfExists(settingsAbs)
	if err != nil {
		return nil, err
	}
	merged, changed, err := MergeHook(raw, HookRegistration{Event: opts.Event, Matcher: opts.Matcher, Command: command})
	if err != nil {
		return nil, exit.Userf("merge settings.json: %v", err)
	}
	if changed {
		if err := s.write(settingsAbs, merged, 0o644); err != nil {
			return nil, err
		}
		res.Created = append(res.Created, "settings.json")
		res.Changed = true
	}

	if s.dryRun {
		return res, nil
	}

	scriptContent, err := os.ReadFile(scriptAbs)
	if err != nil {
		return nil, err
	}
	man, err := loadOrNewClaude(s.dir)
	if err != nil {
		return nil, err
	}
	installedAt := s.clk.Now().UTC().Format(time.RFC3339)
	if existing, ok := man.Find(opts.Name); ok && existing.InstalledAt != "" {
		installedAt = existing.InstalledAt
	}
	man.Upsert(manifest.Capability{
		Name:        opts.Name,
		Type:        string(KindHook),
		Event:       opts.Event,
		Source:      "local",
		Checksum:    checksum(scriptContent),
		Mode:        "own",
		Files:       []string{scriptRel},
		InstalledAt: installedAt,
	})
	if err := manifest.SaveClaude(s.dir, man); err != nil {
		return nil, err
	}
	return res, nil
}

func renderSkill(name, description string) []byte {
	if description == "" {
		description = "TODO: one-line description — this is the trigger Claude Code matches."
	}
	return []byte(fmt.Sprintf(`---
name: %s
description: %s
---

# %s

TODO: describe what this skill does and when Claude Code should use it.
`, name, yamlScalar(description), name))
}

func renderAgent(name, description string) []byte {
	if description == "" {
		description = "TODO: one-line description of when to delegate to this agent."
	}
	return []byte(fmt.Sprintf(`---
name: %s
description: %s
---

You are the %s subagent.

TODO: write the system prompt — responsibilities, constraints, and output format.
`, name, yamlScalar(description), name))
}

// yamlScalar renders s as a YAML-safe scalar, quoting it when necessary (for
// example when it contains a colon). This keeps generated frontmatter valid no
// matter what the user types for a description.
func yamlScalar(s string) string {
	b, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimRight(string(b), "\n")
}

func renderHook(name, event string) []byte {
	return []byte(fmt.Sprintf(`#!/usr/bin/env bash
# %s — teambrain hook for the %s event.
# The hook payload arrives as JSON on stdin; see the Claude Code hooks docs.
set -euo pipefail

# TODO: implement. Exit non-zero to block where the event supports it.
exit 0
`, name, event))
}

func checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFile(abs string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, content, mode)
}

func readIfExists(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

func loadOrNewClaude(dir string) (*manifest.Claude, error) {
	man, err := manifest.LoadClaude(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return manifest.NewClaude(), nil
		}
		return nil, err
	}
	return man, nil
}
