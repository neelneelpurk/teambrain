package mcp

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

// Config is the teambrain-mcp configuration: named vault endpoints and which one
// tools target by default. It is plain JSON, so it can live as a file (per-vault
// keys and ports) or be built in code from the environment.
type Config struct {
	// Default is the vault used when a tool call omits "vault".
	Default string `json:"default,omitempty"`
	// Vaults maps a vault name to its Local REST API endpoint.
	Vaults map[string]obsidianapi.Config `json:"vaults"`
}

// ParseConfig reads and validates a Config from JSON.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse mcp config: %w", err)
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

// SingleVault builds a one-vault Config — the zero-config path from OBSIDIAN_*
// environment variables.
func SingleVault(name string, vc obsidianapi.Config) *Config {
	return &Config{Default: name, Vaults: map[string]obsidianapi.Config{name: vc}}
}

// normalize validates the config and infers the default when it is unambiguous.
func (c *Config) normalize() error {
	if len(c.Vaults) == 0 {
		return fmt.Errorf("no vaults configured")
	}
	if c.Default == "" {
		if len(c.Vaults) != 1 {
			return fmt.Errorf("multiple vaults configured (%v) but no \"default\" set", c.Names())
		}
		for n := range c.Vaults {
			c.Default = n
		}
	}
	if _, ok := c.Vaults[c.Default]; !ok {
		return fmt.Errorf("default vault %q is not among the configured vaults %v", c.Default, c.Names())
	}
	return nil
}

// Names returns the configured vault names, sorted.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Vaults))
	for n := range c.Vaults {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Build constructs one Obsidian REST client per configured vault and returns the
// clients plus the resolved default vault name.
func (c *Config) Build() (map[string]obsidianapi.Client, string, error) {
	if err := c.normalize(); err != nil {
		return nil, "", err
	}
	clients := make(map[string]obsidianapi.Client, len(c.Vaults))
	for name, vc := range c.Vaults {
		client, err := obsidianapi.New(vc)
		if err != nil {
			return nil, "", fmt.Errorf("vault %q: %w", name, err)
		}
		clients[name] = client
	}
	return clients, c.Default, nil
}
