// Package mcp implements the teambrain Obsidian MCP server: a small, read-only
// set of retrieval tools that drive running Obsidian vaults through the Local
// REST API plugin. The tools are teambrain-shaped — they speak the vocabulary of
// the brain and of promotion (notes, headings, backlinks, tags, promotion
// candidates) rather than generic file CRUD, and they route per vault so one
// server can serve a personal brain and several team brains at once. Mutations
// are intentionally absent: changing a vault stays the job of the deterministic
// teambrain CLI.
package mcp

import (
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

// ServerName is the MCP server's self-reported name. It contains "obsidian" so
// that, registered under a matching key, `teambrain doctor` recognizes brain
// retrieval as wired up.
const ServerName = "teambrain-obsidian"

const instructions = `Retrieval tools for teambrain Obsidian vaults, backed by the live app via the
Local REST API. Each configured vault has a name (call list_vaults to see them);
pass "vault" to target one, or omit it for the default. Use search_brain first to
find notes, then read_note (optionally a single heading) to pull only what you
need; widen with list_backlinks. These tools are read-only — to promote notes to
a team, use the teambrain CLI (create-sync / view-sync / commit-sync) or the
promote-to-team skill.`

// NewServer builds the teambrain Obsidian MCP server over a set of named vault
// clients. defaultVault names the vault used when a tool call omits "vault".
func NewServer(vaults map[string]obsidianapi.Client, defaultVault, version string) *mcp.Server {
	names := make([]string, 0, len(vaults))
	for n := range vaults {
		names = append(names, n)
	}
	sort.Strings(names)
	h := &Handlers{vaults: vaults, names: names, def: defaultVault}

	srv := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: version}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_vaults",
		Description: "List the configured vaults (brains) this server can reach, and which is the default. Call this first to learn the vault names to pass as `vault`.",
	}, h.ListVaults)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_brain",
		Description: "Full-text search across a vault (the open Obsidian vault for that endpoint). Returns matching notes with snippets. Use this first when a question depends on vault knowledge.",
	}, h.Search)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_note",
		Description: "Read a note by its vault-relative path. Pass a heading to return only that section — a note is a table of contents, so read just what you need.",
	}, h.ReadNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_active_note",
		Description: "Read the note currently open in Obsidian for a vault — useful for 'summarize what I'm looking at'.",
	}, h.ReadActiveNote)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "note_outline",
		Description: "List a note's headings (its structure), so you can decide which section to read.",
	}, h.Outline)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_backlinks",
		Description: "Find notes that link to a given note via [[wikilinks]], to widen context. Best-effort: matches the [[note prefix, covering alias and heading link forms.",
	}, h.Backlinks)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_notes",
		Description: "List the notes and subfolders under a vault directory (or the vault root), to browse the brain's structure.",
	}, h.ListNotes)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tags",
		Description: "List every tag in a vault with how often it is used, to discover topics and navigate by tag.",
	}, h.ListTags)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "promotion_candidates",
		Description: "List notes tagged for promotion to a team (a `teambrains:` frontmatter property), optionally filtered to one team. Feeds the promote-to-team flow.",
	}, h.PromotionCandidates)

	return srv
}

// --- tool input/output types (schemas are generated from these) ---

// VaultArg is embedded in every per-vault tool input to choose the target vault.
type VaultArg struct {
	Vault string `json:"vault,omitempty" jsonschema:"which configured vault to target (see list_vaults); defaults to the primary"`
}

// SearchInput is the input to search_brain.
type SearchInput struct {
	VaultArg
	Query         string `json:"query" jsonschema:"required,text to search for across the brain"`
	ContextLength int    `json:"context_length,omitempty" jsonschema:"characters of surrounding context per match (default 100)"`
}

// SearchHit is one matching note.
type SearchHit struct {
	Path     string   `json:"path" jsonschema:"vault-relative path of the matching note"`
	Score    float64  `json:"score" jsonschema:"relevance score from Obsidian"`
	Snippets []string `json:"snippets" jsonschema:"matched text with surrounding context"`
}

// SearchOutput is the output of search_brain.
type SearchOutput struct {
	Vault string      `json:"vault" jsonschema:"the vault searched"`
	Hits  []SearchHit `json:"hits" jsonschema:"matching notes, most relevant first"`
}

// ReadNoteInput is the input to read_note.
type ReadNoteInput struct {
	VaultArg
	Path    string `json:"path" jsonschema:"required,vault-relative path to the note (e.g. projects/adr.md)"`
	Heading string `json:"heading,omitempty" jsonschema:"optional heading; return only that section"`
}

// ReadNoteOutput is the output of read_note.
type ReadNoteOutput struct {
	Vault   string `json:"vault" jsonschema:"the vault read from"`
	Path    string `json:"path" jsonschema:"the note's path"`
	Heading string `json:"heading,omitempty" jsonschema:"the heading returned, if a section was requested"`
	Content string `json:"content" jsonschema:"the note (or section) content"`
}

// ActiveNoteInput is the input to read_active_note.
type ActiveNoteInput struct {
	VaultArg
}

// ActiveNoteOutput is the output of read_active_note.
type ActiveNoteOutput struct {
	Vault   string `json:"vault" jsonschema:"the vault whose active note was read"`
	Content string `json:"content" jsonschema:"the active note's content"`
}

// OutlineInput is the input to note_outline.
type OutlineInput struct {
	VaultArg
	Path string `json:"path" jsonschema:"required,vault-relative path to the note"`
}

// HeadingItem is one heading in a note's outline.
type HeadingItem struct {
	Level int    `json:"level" jsonschema:"heading depth (1 for #, 2 for ##, ...)"`
	Title string `json:"title" jsonschema:"heading text"`
}

// OutlineOutput is the output of note_outline.
type OutlineOutput struct {
	Vault    string        `json:"vault" jsonschema:"the vault read from"`
	Path     string        `json:"path" jsonschema:"the note's path"`
	Headings []HeadingItem `json:"headings" jsonschema:"the note's headings, in order"`
}

// BacklinksInput is the input to list_backlinks.
type BacklinksInput struct {
	VaultArg
	Note string `json:"note" jsonschema:"required,note name or path other notes may link to (with or without .md)"`
}

// BacklinksOutput is the output of list_backlinks.
type BacklinksOutput struct {
	Vault     string   `json:"vault" jsonschema:"the vault searched"`
	Note      string   `json:"note" jsonschema:"the note backlinks were sought for"`
	Backlinks []string `json:"backlinks" jsonschema:"paths of notes that link to it"`
}

// ListNotesInput is the input to list_notes.
type ListNotesInput struct {
	VaultArg
	Dir string `json:"dir,omitempty" jsonschema:"vault-relative directory; empty for the vault root"`
}

// ListNotesOutput is the output of list_notes.
type ListNotesOutput struct {
	Vault   string   `json:"vault" jsonschema:"the vault listed"`
	Dir     string   `json:"dir" jsonschema:"the directory listed"`
	Entries []string `json:"entries" jsonschema:"notes and subfolders (subfolders end with /)"`
}

// ListTagsInput is the input to list_tags.
type ListTagsInput struct {
	VaultArg
}

// TagItem is one tag with its usage count.
type TagItem struct {
	Name  string `json:"name" jsonschema:"tag name without the leading #"`
	Count int    `json:"count" jsonschema:"how many times the tag is used"`
}

// ListTagsOutput is the output of list_tags.
type ListTagsOutput struct {
	Vault string    `json:"vault" jsonschema:"the vault inspected"`
	Tags  []TagItem `json:"tags" jsonschema:"tags in the vault"`
}

// PromotionCandidatesInput is the input to promotion_candidates.
type PromotionCandidatesInput struct {
	VaultArg
	Team string `json:"team,omitempty" jsonschema:"optional team name; only notes tagged for this team"`
}

// PromotionCandidate is one note staged-able for promotion.
type PromotionCandidate struct {
	Path  string   `json:"path" jsonschema:"the note's vault-relative path"`
	Teams []string `json:"teams" jsonschema:"team names from the note's teambrains: frontmatter"`
}

// PromotionCandidatesOutput is the output of promotion_candidates.
type PromotionCandidatesOutput struct {
	Vault      string               `json:"vault" jsonschema:"the vault inspected"`
	Candidates []PromotionCandidate `json:"candidates" jsonschema:"notes tagged for promotion"`
}

// ListVaultsInput is the (empty) input to list_vaults.
type ListVaultsInput struct{}

// ListVaultsOutput is the output of list_vaults.
type ListVaultsOutput struct {
	Default string   `json:"default" jsonschema:"the vault used when a tool omits vault"`
	Vaults  []string `json:"vaults" jsonschema:"the configured vault names"`
}
