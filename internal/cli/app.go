package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/viper"
)

// IO bundles the streams a command reads from and writes to. Tests substitute
// buffers; production uses the OS streams.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// BuildInfo carries version metadata injected at link time via -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// String renders a human version line such as "1.2.3 (abc1234, 2026-05-29)".
func (b BuildInfo) String() string {
	v := b.Version
	if v == "" {
		v = "dev"
	}
	switch {
	case b.Commit != "" && b.Date != "":
		return fmt.Sprintf("%s (%s, %s)", v, b.Commit, b.Date)
	case b.Commit != "":
		return fmt.Sprintf("%s (%s)", v, b.Commit)
	default:
		return v
	}
}

// App is the resolved runtime shared across commands: IO streams, build info,
// validated config, and accumulated warnings. A single *App is threaded through
// the command tree via context.
type App struct {
	IO       IO
	Build    BuildInfo
	Config   *Config
	Viper    *viper.Viper
	Command  string
	warnings []string
}

// Warn records a warning. Under --json it is folded into the result envelope;
// otherwise it prints to stderr immediately (unless --quiet).
func (a *App) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.warnings = append(a.warnings, msg)
	if a.Config != nil && a.Config.JSON {
		return
	}
	if a.Config != nil && a.Config.Quiet {
		return
	}
	fmt.Fprintf(a.IO.Err, "warning: %s\n", msg)
}

// Warnings returns a copy of the accumulated warnings.
func (a *App) Warnings() []string {
	return append([]string(nil), a.warnings...)
}

// Emit reports a successful result. Under --json it writes a result envelope to
// stdout (including any warnings); otherwise it invokes human to render
// human-readable output.
func (a *App) Emit(command string, data any, human func(w io.Writer)) error {
	if a.Config != nil && a.Config.JSON {
		return WriteJSON(a.IO.Out, Envelope{
			OK:       true,
			Command:  command,
			Data:     data,
			Warnings: a.warnings,
		})
	}
	if human != nil {
		human(a.IO.Out)
	}
	return nil
}

// appCtxKey is the unexported context key under which the *App is stored.
type appCtxKey struct{}

// withApp returns a context carrying app.
func withApp(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, appCtxKey{}, app)
}

// appFrom extracts the *App from ctx, or nil if absent.
func appFrom(ctx context.Context) *App {
	app, _ := ctx.Value(appCtxKey{}).(*App)
	return app
}
