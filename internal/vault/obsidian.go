package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ObsidianCLI is the enhancement backend that routes operations through the
// Obsidian CLI (which performs IPC to a running Obsidian). Its decisive
// advantage is a link-preserving Move that uses Obsidian's own resolver rather
// than teambrain's documented subset. Operations the CLI does not improve are
// inherited from an embedded FSDirect, so the backend is always fully
// functional.
//
// The exact CLI verb syntax is early-access and may change; verify against
// `obsidian help` at build time. The contract here is pinned by stub tests.
type ObsidianCLI struct {
	*FSDirect
	run func(stdin []byte, args ...string) ([]byte, error)
}

// NewObsidianCLI returns an Obsidian-backed vault rooted at root.
func NewObsidianCLI(root string) (*ObsidianCLI, error) {
	fs, err := NewFSDirect(root)
	if err != nil {
		return nil, err
	}
	o := &ObsidianCLI{FSDirect: fs}
	o.run = o.execObsidian
	return o, nil
}

// Backend identifies this implementation.
func (o *ObsidianCLI) Backend() string { return "obsidian" }

func (o *ObsidianCLI) execObsidian(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("obsidian", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("obsidian %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return out.Bytes(), nil
}

// Read returns a note's content via `obsidian read`.
func (o *ObsidianCLI) Read(rel string) ([]byte, error) {
	out, err := o.run(nil, "read", "--vault", o.Root(), "--path", rel, "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse obsidian read response: %w", err)
	}
	return []byte(resp.Content), nil
}

// Write creates or overwrites a note via `obsidian create`, sending content on
// stdin.
func (o *ObsidianCLI) Write(rel string, content []byte) error {
	_, err := o.run(content, "create", "--vault", o.Root(), "--path", rel, "--json")
	return err
}

// Move relocates a note via `obsidian move`, which rewrites links using
// Obsidian's resolver. The returned report reflects what Obsidian changed.
func (o *ObsidianCLI) Move(srcRel, dstRel string) (*MoveReport, error) {
	out, err := o.run(nil, "move", "--vault", o.Root(), "--from", srcRel, "--to", dstRel, "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Rewrites     []LinkRewrite `json:"rewrites"`
		FilesTouched []string      `json:"files_touched"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse obsidian move response: %w", err)
	}
	return &MoveReport{
		From:         strings.TrimSuffix(srcRel, ".md"),
		To:           strings.TrimSuffix(dstRel, ".md"),
		Rewrites:     resp.Rewrites,
		FilesTouched: resp.FilesTouched,
	}, nil
}

// SearchResult is one hit from Search.
type SearchResult struct {
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

// Search runs a vault search via `obsidian search`.
func (o *ObsidianCLI) Search(query string) ([]SearchResult, error) {
	out, err := o.run(nil, "search", "--vault", o.Root(), "--query", query, "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse obsidian search response: %w", err)
	}
	return resp.Results, nil
}

// Backlinks returns the notes linking to rel via `obsidian backlinks`.
func (o *ObsidianCLI) Backlinks(rel string) ([]string, error) {
	out, err := o.run(nil, "backlinks", "--vault", o.Root(), "--path", rel, "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Backlinks []string `json:"backlinks"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse obsidian backlinks response: %w", err)
	}
	return resp.Backlinks, nil
}

// Outline returns the heading outline of a note via `obsidian outline`.
func (o *ObsidianCLI) Outline(rel string) ([]string, error) {
	out, err := o.run(nil, "outline", "--vault", o.Root(), "--path", rel, "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Headings []string `json:"headings"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse obsidian outline response: %w", err)
	}
	return resp.Headings, nil
}

// Unresolved returns the vault's unresolved (dangling) link targets via
// `obsidian unresolved`.
func (o *ObsidianCLI) Unresolved() ([]string, error) {
	out, err := o.run(nil, "unresolved", "--vault", o.Root(), "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Unresolved []string `json:"unresolved"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse obsidian unresolved response: %w", err)
	}
	return resp.Unresolved, nil
}

// SetProperty sets a frontmatter property via `obsidian property set`.
func (o *ObsidianCLI) SetProperty(rel, key, value string) error {
	_, err := o.run(nil, "property", "set", "--vault", o.Root(), "--path", rel, "--key", key, "--value", value, "--json")
	return err
}
