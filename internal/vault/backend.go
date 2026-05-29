package vault

import "os/exec"

// Backend names a vault access strategy.
type Backend string

const (
	// BackendFS reads and writes vault files directly. Always available.
	BackendFS Backend = "fs"
	// BackendObsidian routes operations through the Obsidian CLI.
	BackendObsidian Backend = "obsidian"
	// BackendAuto picks obsidian when detected, otherwise fs.
	BackendAuto Backend = "auto"
)

// Warn is a logging callback used to report backend selection and desync risks.
type Warn func(format string, args ...any)

// DetectObsidian reports whether the Obsidian CLI is available on PATH. It is
// the default detector for Open.
func DetectObsidian() bool {
	_, err := exec.LookPath("obsidian")
	return err == nil
}

// Open returns a Vault for root using the requested backend. For BackendAuto it
// chooses obsidian when detect reports it available, otherwise fs — and always
// logs the choice via warn. For an explicit obsidian request that is undetected
// it degrades gracefully to fs with a warning. For an explicit fs request while
// obsidian is detected, it warns that direct writes may desync a running vault.
//
// detect and warn may be nil (defaults are used / logging is skipped).
func Open(backend Backend, root string, detect func() bool, warn Warn) (Vault, error) {
	if detect == nil {
		detect = DetectObsidian
	}
	if warn == nil {
		warn = func(string, ...any) {}
	}

	switch backend {
	case BackendObsidian:
		if !detect() {
			warn("obsidian backend requested but the Obsidian CLI was not found; using fs")
			return NewFSDirect(root)
		}
		warn("using obsidian backend")
		return NewObsidianCLI(root)

	case BackendAuto:
		if detect() {
			warn("using obsidian backend (auto-detected)")
			return NewObsidianCLI(root)
		}
		return NewFSDirect(root)

	default: // BackendFS and anything unexpected
		if detect() {
			warn("Obsidian CLI detected but using fs backend; direct writes may desync a running vault")
		}
		return NewFSDirect(root)
	}
}
