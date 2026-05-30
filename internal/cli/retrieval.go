package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Brain retrieval is powered by Obsidian (MCP preferred, else the CLI). teambrain
// deliberately does not reimplement search; instead the search-brain skill drives
// Obsidian, and these helpers report/enforce that one of them is present.
const (
	retrievalMCP  = "obsidian-mcp"
	retrievalCLI  = "obsidian-cli"
	retrievalNone = "unavailable"

	retrievalSetupHint = "run the bundled teambrain-mcp (or another Obsidian MCP), or install the Obsidian CLI — the search-brain skill needs one"
)

// detectObsidianCLI reports whether the Obsidian CLI is on PATH. The
// search-brain skill can drive it as a retrieval fallback when no Obsidian MCP
// is configured; teambrain itself reads and writes vaults directly.
func detectObsidianCLI() bool {
	_, err := exec.LookPath("obsidian")
	return err == nil
}

// detectObsidianMCP heuristically reports whether an Obsidian MCP server is
// configured, by scanning .mcp.json in dir and ~/.claude.json for a server whose
// name mentions "obsidian". teambrain cannot observe Claude's live MCP
// connections, so this is a config-file proxy; the search-brain skill performs
// the real runtime check.
func detectObsidianMCP(dir string) bool {
	candidates := []string{filepath.Join(dir, ".mcp.json")}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".claude.json"))
	}
	for _, p := range candidates {
		if mcpConfigHasObsidian(p) {
			return true
		}
	}
	return false
}

func mcpConfigHasObsidian(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}
	for name := range cfg.MCPServers {
		if strings.Contains(strings.ToLower(name), "obsidian") {
			return true
		}
	}
	return false
}

// retrievalStatus reports the active brain-retrieval path and whether retrieval
// is available at all (Obsidian MCP preferred, else the Obsidian CLI).
func retrievalStatus(dir string) (path string, available bool) {
	switch {
	case detectObsidianMCP(dir):
		return retrievalMCP, true
	case detectObsidianCLI():
		return retrievalCLI, true
	default:
		return retrievalNone, false
	}
}
