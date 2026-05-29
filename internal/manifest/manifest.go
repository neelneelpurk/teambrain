// Package manifest defines teambrain's two plain-JSON manifests and their
// read/write helpers. Both are named ".teambrain.json":
//
//   - the Root manifest lives at the vault root and records the vault's role and
//     its 1:1 team binding;
//   - the Claude manifest lives inside a ".claude" directory (in a vault or a
//     code repo) and records which capabilities teambrain owns there, with
//     checksums for tamper detection.
//
// The files are human-readable and authoritative; teambrain keeps no database.
package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// Version is the current manifest schema version.
const Version = 1

// FileName is the on-disk name of both manifests.
const FileName = ".teambrain.json"

// Vault roles.
const (
	RolePersonal = "personal"
	RoleTeam     = "team"
)

// Root is the vault-root manifest (`<vault>/.teambrain.json`). A personal vault
// may bind to several team vaults (1:n); each binding carries a unique Name that
// notes reference via their `teambrains:` frontmatter property.
type Root struct {
	Version int           `json:"version"`
	Vault   string        `json:"vault"`
	Teams   []TeamBinding `json:"teams,omitempty"`
}

// TeamBinding is a named pointer from a personal vault to one team vault.
type TeamBinding struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Remote  string `json:"remote,omitempty"`
	BoundAt string `json:"bound_at,omitempty"`
}

// rootJSON is the on-disk shape, including the legacy single `team` field for
// backward-compatible reads.
type rootJSON struct {
	Version int           `json:"version"`
	Vault   string        `json:"vault"`
	Teams   []TeamBinding `json:"teams,omitempty"`
	Team    *TeamBinding  `json:"team,omitempty"` // legacy (pre-1.n)
}

// UnmarshalJSON reads the on-disk shape and migrates a legacy single `team` into
// the Teams list (named after its path/remote) when no Teams are present.
func (r *Root) UnmarshalJSON(data []byte) error {
	var raw rootJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Version = raw.Version
	r.Vault = raw.Vault
	r.Teams = raw.Teams
	if len(r.Teams) == 0 && raw.Team != nil {
		legacy := *raw.Team
		if legacy.Name == "" {
			legacy.Name = "team"
		}
		r.Teams = []TeamBinding{legacy}
	}
	return nil
}

// MarshalJSON writes only the current shape (never the legacy `team` field).
func (r *Root) MarshalJSON() ([]byte, error) {
	return json.Marshal(rootJSON{Version: r.Version, Vault: r.Vault, Teams: r.Teams})
}

// NewRoot returns an unbound root manifest for the given role.
func NewRoot(role string) *Root {
	return &Root{Version: Version, Vault: role}
}

// IsBound reports whether at least one team vault is bound.
func (r *Root) IsBound() bool {
	return len(r.Teams) > 0
}

// Team returns the binding with the given name and whether it was found.
func (r *Root) Team(name string) (TeamBinding, bool) {
	for _, t := range r.Teams {
		if t.Name == name {
			return t, true
		}
	}
	return TeamBinding{}, false
}

// UpsertTeam adds binding, replacing any existing team with the same name
// (preserving its position).
func (r *Root) UpsertTeam(binding TeamBinding) {
	for i := range r.Teams {
		if r.Teams[i].Name == binding.Name {
			r.Teams[i] = binding
			return
		}
	}
	r.Teams = append(r.Teams, binding)
}

// RemoveTeam deletes the named team, reporting whether it existed.
func (r *Root) RemoveTeam(name string) bool {
	for i := range r.Teams {
		if r.Teams[i].Name == name {
			r.Teams = append(r.Teams[:i], r.Teams[i+1:]...)
			return true
		}
	}
	return false
}

// Claude is the .claude ownership manifest (`.claude/.teambrain.json`).
type Claude struct {
	Version      int          `json:"version"`
	Capabilities []Capability `json:"capabilities"`
}

// Capability records one teambrain-owned capability in a .claude directory.
type Capability struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Event       string   `json:"event,omitempty"`
	Source      string   `json:"source,omitempty"`
	Checksum    string   `json:"checksum,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Files       []string `json:"files,omitempty"`
	InstalledAt string   `json:"installed_at,omitempty"`
}

// NewClaude returns an empty ownership manifest with a non-nil capability slice
// so it marshals as [] rather than null.
func NewClaude() *Claude {
	return &Claude{Version: Version, Capabilities: []Capability{}}
}

// Find returns the capability with the given name and whether it was found.
func (c *Claude) Find(name string) (Capability, bool) {
	for _, entry := range c.Capabilities {
		if entry.Name == name {
			return entry, true
		}
	}
	return Capability{}, false
}

// Upsert inserts entry, replacing any existing capability with the same name
// (preserving its position).
func (c *Claude) Upsert(entry Capability) {
	for i := range c.Capabilities {
		if c.Capabilities[i].Name == entry.Name {
			c.Capabilities[i] = entry
			return
		}
	}
	c.Capabilities = append(c.Capabilities, entry)
}

// Remove deletes the named capability, reporting whether it existed.
func (c *Claude) Remove(name string) bool {
	for i := range c.Capabilities {
		if c.Capabilities[i].Name == name {
			c.Capabilities = append(c.Capabilities[:i], c.Capabilities[i+1:]...)
			return true
		}
	}
	return false
}

// LoadRoot reads the root manifest from vaultRoot. A missing file yields an
// error satisfying errors.Is(err, fs.ErrNotExist).
func LoadRoot(vaultRoot string) (*Root, error) {
	var r Root
	if err := loadJSON(filepath.Join(vaultRoot, FileName), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// SaveRoot writes the root manifest into vaultRoot.
func SaveRoot(vaultRoot string, r *Root) error {
	return saveJSON(filepath.Join(vaultRoot, FileName), r)
}

// LoadClaude reads the ownership manifest from a .claude directory.
func LoadClaude(claudeDir string) (*Claude, error) {
	var c Claude
	if err := loadJSON(filepath.Join(claudeDir, FileName), &c); err != nil {
		return nil, err
	}
	if c.Capabilities == nil {
		c.Capabilities = []Capability{}
	}
	return &c, nil
}

// SaveClaude writes the ownership manifest into a .claude directory.
func SaveClaude(claudeDir string, c *Claude) error {
	return saveJSON(filepath.Join(claudeDir, FileName), c)
}

func loadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Marshal renders v as teambrain's canonical JSON: two-space indent, no HTML
// escaping, trailing newline. It is the single source of the on-disk format, so
// manifests written via the Vault interface match those written via SaveRoot.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// saveJSON writes v as canonical JSON, creating parent directories as needed.
func saveJSON(path string, v any) error {
	data, err := Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
