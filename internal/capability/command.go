package capability

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/manifest"
)

// NewCommand creates .claude/commands/<name>.md, a Claude Code slash command,
// with valid frontmatter and a stub prompt. It errors if the command exists.
func (s *Store) NewCommand(name, description string) (*NewResult, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(filepath.Join("commands", name+".md"))
	abs := filepath.Join(s.dir, filepath.FromSlash(rel))
	if fileExists(abs) {
		return nil, exit.Userf("command %q already exists at %s", name, rel)
	}
	if err := s.write(abs, renderCommand(name, description), 0o644); err != nil {
		return nil, err
	}
	return &NewResult{Name: name, Kind: string(KindCommand), Path: rel, Created: []string{rel}, Changed: true}, nil
}

func renderCommand(name, description string) []byte {
	if description == "" {
		description = "TODO: one-line description of what this command does."
	}
	return []byte(fmt.Sprintf(`---
description: %s
---

# /%s

TODO: write the command prompt. Use $ARGUMENTS for user-supplied input.
`, yamlScalar(description), name))
}

// Drift records a teambrain-owned file whose on-disk content no longer matches
// the checksum recorded at install time — tamper or accidental edit.
type Drift struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Expected string `json:"expected"`
	Actual   string `json:"actual,omitempty"`
	Reason   string `json:"reason"` // "modified" | "missing"
}

// CheckDrift recomputes the checksum of every owned capability's primary file
// and reports any that have changed or gone missing. Link-mode capabilities are
// skipped (they intentionally point at a source). It is the basis of doctor's
// tamper detection.
func (s *Store) CheckDrift() ([]Drift, error) {
	man, err := manifest.LoadClaude(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var drifts []Drift
	for _, c := range man.Capabilities {
		if c.Checksum == "" || c.Mode == "link" {
			continue
		}
		primary := primaryRel(Kind(c.Type), c.Name)
		data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(primary)))
		if err != nil {
			drifts = append(drifts, Drift{Name: c.Name, File: primary, Expected: c.Checksum, Reason: "missing"})
			continue
		}
		if actual := checksum(data); actual != c.Checksum {
			drifts = append(drifts, Drift{Name: c.Name, File: primary, Expected: c.Checksum, Actual: actual, Reason: "modified"})
		}
	}
	return drifts, nil
}
