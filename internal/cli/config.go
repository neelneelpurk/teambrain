package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/neelneelpurk/teambrain/internal/exit"
)

// EnvPrefix is the prefix for all teambrain environment variables, e.g.
// TEAMBRAIN_VAULT_BACKEND.
const EnvPrefix = "TEAMBRAIN"

// Backend names a vault access strategy.
type Backend string

const (
	// BackendFS reads and writes vault files directly on disk. Always available.
	BackendFS Backend = "fs"
	// BackendObsidian routes operations through the Obsidian CLI for
	// link-preserving moves and richer queries. Requires a running Obsidian.
	BackendObsidian Backend = "obsidian"
	// BackendAuto picks obsidian when detected, otherwise fs.
	BackendAuto Backend = "auto"
)

// validBackends is the closed set accepted for vault_backend.
var validBackends = map[string]bool{
	string(BackendFS):       true,
	string(BackendObsidian): true,
	string(BackendAuto):     true,
}

// configKeys lists every configuration key. Each is bound to an environment
// variable so that Unmarshal observes env overrides (Viper's AutomaticEnv alone
// does not feed Unmarshal reliably).
var configKeys = []string{
	"vault_backend",
	"json",
	"dry_run",
	"yes",
	"verbose",
	"quiet",
	"no_color",
	"personal_vault",
}

// Config is the fully resolved global configuration after applying precedence:
// explicit flags > environment (TEAMBRAIN_*) > config file > defaults.
type Config struct {
	VaultBackend string `mapstructure:"vault_backend"`
	JSON         bool   `mapstructure:"json"`
	DryRun       bool   `mapstructure:"dry_run"`
	Yes          bool   `mapstructure:"yes"`
	Verbose      bool   `mapstructure:"verbose"`
	Quiet        bool   `mapstructure:"quiet"`
	NoColor      bool   `mapstructure:"no_color"`
	// PersonalVault, when set, is the personal-brain vault path. import uses it
	// (and its bound team vault) as default capability sources.
	PersonalVault string `mapstructure:"personal_vault"`
}

// ConfigDir returns teambrain's configuration directory following the XDG Base
// Directory spec: $XDG_CONFIG_HOME/teambrain when set, else ~/.config/teambrain.
// XDG semantics are honored on every platform for predictability.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "teambrain")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "teambrain")
	}
	return filepath.Join(home, ".config", "teambrain")
}

// NewViper builds a Viper configured with teambrain's defaults, env binding, and
// config-file search path. It does not read the file yet; LoadConfig does.
func NewViper() *viper.Viper {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(ConfigDir())

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	for _, key := range configKeys {
		// Errors are impossible for non-empty keys; ignore deliberately.
		_ = v.BindEnv(key)
	}

	v.SetDefault("vault_backend", string(BackendAuto))
	v.SetDefault("json", false)
	v.SetDefault("dry_run", false)
	v.SetDefault("yes", false)
	v.SetDefault("verbose", false)
	v.SetDefault("quiet", false)
	v.SetDefault("no_color", false)
	v.SetDefault("personal_vault", "")

	return v
}

// LoadConfig reads the config file (if present), applies env overrides, and
// returns a validated Config. A missing config file is not an error.
func LoadConfig(v *viper.Viper) (*Config, error) {
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, exit.Userf("read config file: %v", err).
				WithHint("check the config file at " + v.ConfigFileUsed())
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, exit.Userf("parse configuration: %v", err).
			WithHint("check the config file at " + v.ConfigFileUsed())
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks invariants that the type system cannot.
func (c *Config) Validate() error {
	if !validBackends[c.VaultBackend] {
		return exit.Userf("invalid vault_backend %q: want one of fs, obsidian, auto", c.VaultBackend).
			WithHint("set --vault-backend, TEAMBRAIN_VAULT_BACKEND, or vault_backend in config")
	}
	return nil
}
