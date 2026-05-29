package vault

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// fmFence is the YAML frontmatter delimiter line.
var fmFence = []byte("---")

// Document is a parsed Markdown note: an optional ordered YAML frontmatter block
// plus the body. It round-trips key order and the body verbatim, so editing a
// single property never reflows unrelated content.
type Document struct {
	// HasFrontmatter reports whether the note began with a YAML frontmatter
	// block (or one was created via Set).
	HasFrontmatter bool
	// Body is everything after the frontmatter block, byte-for-byte.
	Body []byte

	mapping *yaml.Node // a MappingNode, or nil when there is no frontmatter
}

// ParseDocument splits raw into frontmatter and body. A leading line of exactly
// "---" opens a block that ends at the next such line. Without it, the whole
// input is the body and HasFrontmatter is false.
func ParseDocument(raw []byte) (*Document, error) {
	d := &Document{Body: raw}

	fm, body, ok := splitFrontmatter(raw)
	if !ok {
		return d, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(fm, &doc); err != nil {
		return nil, err
	}

	d.HasFrontmatter = true
	d.Body = body
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		d.mapping = doc.Content[0]
	} else {
		// Empty or non-mapping frontmatter: start from an empty mapping so Set
		// behaves predictably.
		d.mapping = &yaml.Node{Kind: yaml.MappingNode}
	}
	return d, nil
}

// splitFrontmatter returns the YAML between the opening and closing fences and
// the remaining body. ok is false when the input does not open with a fence.
func splitFrontmatter(raw []byte) (fm, body []byte, ok bool) {
	if !bytes.HasPrefix(raw, fmFence) {
		return nil, nil, false
	}
	// The opening fence must be its own line (terminated by LF or CRLF).
	rest := raw[len(fmFence):]
	hasLF := len(rest) > 0 && rest[0] == '\n'
	hasCRLF := len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n'
	if !hasLF && !hasCRLF {
		return nil, nil, false
	}

	// Find the closing fence: a line consisting solely of "---".
	lines := bytes.SplitAfter(raw, []byte("\n"))
	if len(lines) < 2 {
		return nil, nil, false
	}
	var fmBuf bytes.Buffer
	for i := 1; i < len(lines); i++ {
		trimmed := bytes.TrimRight(lines[i], "\r\n")
		if bytes.Equal(trimmed, fmFence) {
			// Body is everything after this closing fence line.
			consumed := 0
			for j := 0; j <= i; j++ {
				consumed += len(lines[j])
			}
			return fmBuf.Bytes(), raw[consumed:], true
		}
		fmBuf.Write(lines[i])
	}
	// No closing fence: treat as no frontmatter (avoid mangling).
	return nil, nil, false
}

// Get returns the scalar string value of key and whether it was present.
func (d *Document) Get(key string) (string, bool) {
	if d.mapping == nil {
		return "", false
	}
	for i := 0; i+1 < len(d.mapping.Content); i += 2 {
		if d.mapping.Content[i].Value == key {
			return d.mapping.Content[i+1].Value, true
		}
	}
	return "", false
}

// Keys returns the frontmatter keys in document order.
func (d *Document) Keys() []string {
	if d.mapping == nil {
		return nil
	}
	keys := make([]string, 0, len(d.mapping.Content)/2)
	for i := 0; i < len(d.mapping.Content); i += 2 {
		keys = append(keys, d.mapping.Content[i].Value)
	}
	return keys
}

// Set assigns key to value, updating in place if the key exists (preserving its
// position) or appending otherwise. On a frontmatter-less document it creates
// the block.
func (d *Document) Set(key string, value any) error {
	if d.mapping == nil {
		d.mapping = &yaml.Node{Kind: yaml.MappingNode}
		d.HasFrontmatter = true
	}

	valNode := &yaml.Node{}
	if err := valNode.Encode(value); err != nil {
		return err
	}

	for i := 0; i+1 < len(d.mapping.Content); i += 2 {
		if d.mapping.Content[i].Value == key {
			d.mapping.Content[i+1] = valNode
			return nil
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	d.mapping.Content = append(d.mapping.Content, keyNode, valNode)
	return nil
}

// Render reassembles the note. When frontmatter is present it is re-emitted
// between fences followed by the verbatim body.
func (d *Document) Render() ([]byte, error) {
	if !d.HasFrontmatter || d.mapping == nil || len(d.mapping.Content) == 0 {
		return d.Body, nil
	}

	var yamlBuf bytes.Buffer
	enc := yaml.NewEncoder(&yamlBuf)
	enc.SetIndent(2)
	if err := enc.Encode(d.mapping); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(yamlBuf.Bytes())
	out.WriteString("---\n")
	out.Write(d.Body)
	return out.Bytes(), nil
}
