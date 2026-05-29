package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

// Handlers implement the MCP tools over a set of named, read-only Obsidian REST
// clients (one per vault).
type Handlers struct {
	vaults map[string]obsidianapi.Client
	names  []string // configured vault names, sorted
	def    string   // default vault when a call omits one
}

// resolve picks the client for name (or the default when empty). On an unknown
// name it returns a tool-error result so the model can recover.
func (h *Handlers) resolve(name string) (obsidianapi.Client, string, *mcp.CallToolResult) {
	if name == "" {
		name = h.def
	}
	c, ok := h.vaults[name]
	if !ok {
		return nil, name, toolError("unknown vault %q; configured vaults: %s", name, strings.Join(h.names, ", "))
	}
	return c, name, nil
}

// ListVaults implements list_vaults.
func (h *Handlers) ListVaults(_ context.Context, _ *mcp.CallToolRequest, _ ListVaultsInput) (*mcp.CallToolResult, ListVaultsOutput, error) {
	out := ListVaultsOutput{Default: h.def, Vaults: h.names}
	return textResult(fmt.Sprintf("%d vault(s): %s (default: %s)", len(h.names), strings.Join(h.names, ", "), h.def)), out, nil
}

// Search implements search_brain.
func (h *Handlers) Search(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, SearchOutput{}, nil
	}
	if strings.TrimSpace(in.Query) == "" {
		return toolError("query must not be empty"), SearchOutput{}, nil
	}
	res, err := client.Search(ctx, in.Query, in.ContextLength)
	if err != nil {
		return toolError("search failed: %v", err), SearchOutput{}, nil
	}
	out := SearchOutput{Vault: vault, Hits: make([]SearchHit, 0, len(res))}
	for _, r := range res {
		snippets := make([]string, 0, len(r.Matches))
		for _, m := range r.Matches {
			if s := strings.TrimSpace(m.Context); s != "" {
				snippets = append(snippets, s)
			}
		}
		out.Hits = append(out.Hits, SearchHit{Path: r.Filename, Score: r.Score, Snippets: snippets})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d note(s) in %q matching %q.", len(out.Hits), vault, in.Query)
	for _, hit := range out.Hits {
		fmt.Fprintf(&b, "\n- %s", hit.Path)
	}
	return textResult(b.String()), out, nil
}

// ReadNote implements read_note.
func (h *Handlers) ReadNote(ctx context.Context, _ *mcp.CallToolRequest, in ReadNoteInput) (*mcp.CallToolResult, ReadNoteOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, ReadNoteOutput{}, nil
	}
	if strings.TrimSpace(in.Path) == "" {
		return toolError("path must not be empty"), ReadNoteOutput{}, nil
	}
	content, err := client.ReadNote(ctx, in.Path)
	if err != nil {
		return toolError("read %q: %v", in.Path, err), ReadNoteOutput{}, nil
	}
	out := ReadNoteOutput{Vault: vault, Path: in.Path, Content: content}
	if in.Heading != "" {
		section, ok := extractSection(content, in.Heading)
		if !ok {
			return toolError("heading %q not found in %q", in.Heading, in.Path), ReadNoteOutput{}, nil
		}
		out.Heading = in.Heading
		out.Content = section
	}
	return textResult(out.Content), out, nil
}

// ReadActiveNote implements read_active_note.
func (h *Handlers) ReadActiveNote(ctx context.Context, _ *mcp.CallToolRequest, in ActiveNoteInput) (*mcp.CallToolResult, ActiveNoteOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, ActiveNoteOutput{}, nil
	}
	content, err := client.ReadActiveNote(ctx)
	if err != nil {
		return toolError("read active note in %q: %v", vault, err), ActiveNoteOutput{}, nil
	}
	return textResult(content), ActiveNoteOutput{Vault: vault, Content: content}, nil
}

// Outline implements note_outline.
func (h *Handlers) Outline(ctx context.Context, _ *mcp.CallToolRequest, in OutlineInput) (*mcp.CallToolResult, OutlineOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, OutlineOutput{}, nil
	}
	if strings.TrimSpace(in.Path) == "" {
		return toolError("path must not be empty"), OutlineOutput{}, nil
	}
	content, err := client.ReadNote(ctx, in.Path)
	if err != nil {
		return toolError("read %q: %v", in.Path, err), OutlineOutput{}, nil
	}
	headings := parseHeadings(content)
	out := OutlineOutput{Vault: vault, Path: in.Path, Headings: headings}

	var b strings.Builder
	fmt.Fprintf(&b, "%s — %d heading(s):", in.Path, len(headings))
	for _, hd := range headings {
		fmt.Fprintf(&b, "\n%s %s", strings.Repeat("#", hd.Level), hd.Title)
	}
	return textResult(b.String()), out, nil
}

// Backlinks implements list_backlinks.
func (h *Handlers) Backlinks(ctx context.Context, _ *mcp.CallToolRequest, in BacklinksInput) (*mcp.CallToolResult, BacklinksOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, BacklinksOutput{}, nil
	}
	name := strings.TrimSuffix(strings.TrimSpace(in.Note), ".md")
	if name == "" {
		return toolError("note must not be empty"), BacklinksOutput{}, nil
	}
	// The Local REST API exposes no link graph, so match the [[note prefix
	// against each file's content via JsonLogic. This covers [[note]],
	// [[note|alias]], and [[note#heading]].
	query := map[string]any{"in": []any{"[[" + name, map[string]any{"var": "content"}}}
	res, err := client.SearchJSONLogic(ctx, query)
	if err != nil {
		return toolError("backlink search failed: %v", err), BacklinksOutput{}, nil
	}
	links := make([]string, 0, len(res))
	for _, r := range res {
		links = append(links, r.Filename)
	}
	sort.Strings(links)
	out := BacklinksOutput{Vault: vault, Note: name, Backlinks: links}
	return textResult(fmt.Sprintf("%d note(s) in %q link to %q.", len(links), vault, name)), out, nil
}

// ListNotes implements list_notes.
func (h *Handlers) ListNotes(ctx context.Context, _ *mcp.CallToolRequest, in ListNotesInput) (*mcp.CallToolResult, ListNotesOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, ListNotesOutput{}, nil
	}
	entries, err := client.ListFiles(ctx, in.Dir)
	if err != nil {
		return toolError("list %q: %v", in.Dir, err), ListNotesOutput{}, nil
	}
	out := ListNotesOutput{Vault: vault, Dir: in.Dir, Entries: entries}
	return textResult(fmt.Sprintf("%d entr(y/ies) under %q in %q.", len(entries), in.Dir, vault)), out, nil
}

// ListTags implements list_tags.
func (h *Handlers) ListTags(ctx context.Context, _ *mcp.CallToolRequest, in ListTagsInput) (*mcp.CallToolResult, ListTagsOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, ListTagsOutput{}, nil
	}
	tags, err := client.ListTags(ctx)
	if err != nil {
		return toolError("list tags in %q: %v", vault, err), ListTagsOutput{}, nil
	}
	out := ListTagsOutput{Vault: vault, Tags: make([]TagItem, 0, len(tags))}
	for _, tg := range tags {
		out.Tags = append(out.Tags, TagItem{Name: tg.Name, Count: tg.Count})
	}
	return textResult(fmt.Sprintf("%d tag(s) in %q.", len(out.Tags), vault)), out, nil
}

// PromotionCandidates implements promotion_candidates.
func (h *Handlers) PromotionCandidates(ctx context.Context, _ *mcp.CallToolRequest, in PromotionCandidatesInput) (*mcp.CallToolResult, PromotionCandidatesOutput, error) {
	client, vault, errRes := h.resolve(in.Vault)
	if errRes != nil {
		return errRes, PromotionCandidatesOutput{}, nil
	}
	// Files whose frontmatter has a `teambrains` property: the var evaluates to
	// the property's value (the team list), and the plugin returns only files
	// where that is truthy.
	query := map[string]any{"var": "frontmatter.teambrains"}
	res, err := client.SearchJSONLogic(ctx, query)
	if err != nil {
		return toolError("candidate search failed: %v", err), PromotionCandidatesOutput{}, nil
	}
	out := PromotionCandidatesOutput{Vault: vault, Candidates: make([]PromotionCandidate, 0, len(res))}
	for _, r := range res {
		teams := decodeTeams(r.Result)
		if in.Team != "" && !contains(teams, in.Team) {
			continue
		}
		out.Candidates = append(out.Candidates, PromotionCandidate{Path: r.Filename, Teams: teams})
	}
	sort.Slice(out.Candidates, func(i, j int) bool { return out.Candidates[i].Path < out.Candidates[j].Path })

	var b strings.Builder
	if in.Team != "" {
		fmt.Fprintf(&b, "%d note(s) in %q tagged for team %q.", len(out.Candidates), vault, in.Team)
	} else {
		fmt.Fprintf(&b, "%d note(s) in %q tagged for promotion.", len(out.Candidates), vault)
	}
	for _, c := range out.Candidates {
		fmt.Fprintf(&b, "\n- %s → %s", c.Path, strings.Join(c.Teams, ", "))
	}
	return textResult(b.String()), out, nil
}

// --- pure helpers ---

// decodeTeams interprets a teambrains frontmatter value, which Obsidian may
// return as a list or a single string.
func decodeTeams(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil && one != "" {
		return []string{one}
	}
	return nil
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// parseHeadings extracts ATX headings (#..######) from Markdown, ignoring lines
// inside fenced code blocks.
func parseHeadings(content string) []HeadingItem {
	var headings []HeadingItem
	fenced := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if level, title, ok := atxHeading(line); ok {
			headings = append(headings, HeadingItem{Level: level, Title: title})
		}
	}
	return headings
}

// atxHeading parses a single ATX heading line, returning its level and title.
func atxHeading(line string) (level int, title string, ok bool) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return 0, "", false
	}
	if i >= len(line) || (line[i] != ' ' && line[i] != '\t') {
		return 0, "", false
	}
	return i, strings.TrimSpace(line[i:]), true
}

// extractSection returns the slice of content from the heading matching name
// (case-insensitive) through to the next heading of equal-or-higher level,
// inclusive of the heading line.
func extractSection(content, name string) (string, bool) {
	lines := strings.Split(content, "\n")
	want := strings.ToLower(strings.TrimSpace(name))
	start, startLevel := -1, 0
	fenced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		level, title, ok := atxHeading(line)
		if !ok {
			continue
		}
		if start == -1 {
			if strings.ToLower(title) == want {
				start, startLevel = i, level
			}
			continue
		}
		if level <= startLevel {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start == -1 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// textResult wraps a human-readable string as tool content; the typed Out still
// rides along as structured content.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// toolError reports a tool-level failure the model can read and recover from.
func toolError(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}
