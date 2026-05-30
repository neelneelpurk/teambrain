// Command teambrain-mcp is the teambrain Obsidian MCP server. It bridges Claude
// Code to one or more running Obsidian vaults through the Local REST API
// community plugin, exposing read-only, teambrain-shaped retrieval tools (search
// the brain, read a note or heading, the active note, outline, backlinks, tags,
// promotion candidates), routed per vault.
//
// # Configuration
//
// Multi-vault: a JSON file at $TEAMBRAIN_MCP_CONFIG (else
// $XDG_CONFIG_HOME/teambrain/mcp.json) mapping vault names to endpoints:
//
//	{
//	  "default": "personal",
//	  "vaults": {
//	    "personal": {"api_key": "...", "port": 27124},
//	    "eng":      {"api_key": "...", "port": 27125}
//	  }
//	}
//
// Single vault (zero config): the OBSIDIAN_* environment variables —
// OBSIDIAN_API_KEY (required), OBSIDIAN_HOST (default 127.0.0.1), OBSIDIAN_PORT
// (default 27124), OBSIDIAN_PROTOCOL (https|http), OBSIDIAN_VERIFY_TLS ("true"),
// and OBSIDIAN_CA_CERT (path to the plugin's certificate, for verified TLS).
//
// It speaks MCP over stdio, so register it in Claude Code under a key containing
// "obsidian" and `teambrain doctor` will see retrieval as wired up.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neelneelpurk/teambrain/internal/mcp"
	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

// Populated via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := loadConfig(logger)
	if err != nil {
		logger.Error("configuration error: " + err.Error())
		os.Exit(1)
	}
	clients, def, err := cfg.Build()
	if err != nil {
		logger.Error("cannot start: "+err.Error(), "hint", "set OBSIDIAN_API_KEY (or a vault config) from the Local REST API plugin settings")
		os.Exit(1)
	}

	srv := mcp.NewServer(clients, def, version)
	logger.Info("teambrain-obsidian MCP starting",
		"version", version, "commit", commit, "date", date, "vaults", cfg.Names(), "default", def)

	if err := srv.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

// loadConfig prefers a JSON vault-config file, falling back to a single vault
// built from the OBSIDIAN_* environment.
func loadConfig(logger *slog.Logger) (*mcp.Config, error) {
	if path := configPath(); path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			logger.Info("loaded vault config", "path", path)
			return mcp.ParseConfig(data)
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	return mcp.SingleVault("default", configFromEnv()), nil
}

// configPath resolves the vault-config file location: an explicit override, else
// the teambrain config dir.
func configPath() string {
	if p := os.Getenv("TEAMBRAIN_MCP_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		base = dir
	}
	return filepath.Join(base, "teambrain", "mcp.json")
}

// configFromEnv reads a single vault's endpoint from the OBSIDIAN_* environment.
func configFromEnv() obsidianapi.Config {
	cfg := obsidianapi.Config{
		APIKey:    os.Getenv("OBSIDIAN_API_KEY"),
		Protocol:  os.Getenv("OBSIDIAN_PROTOCOL"),
		Host:      os.Getenv("OBSIDIAN_HOST"),
		CACert:    os.Getenv("OBSIDIAN_CA_CERT"),
		VerifyTLS: os.Getenv("OBSIDIAN_VERIFY_TLS") == "true",
	}
	if p := os.Getenv("OBSIDIAN_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			cfg.Port = port
		}
	}
	return cfg
}
