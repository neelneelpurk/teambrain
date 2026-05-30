// Package cli implements teambrain's command-line interface: the Cobra command
// tree, configuration loading, the --json result envelope, and the top-level
// error-to-exit-code mapping that realizes the documented UX contract.
package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/neelneelpurk/teambrain/internal/exit"
)

const rootLong = `teambrain manages two Obsidian vaults — a private personal brain and a
shared team brain — that Claude Code can read, extend, and share between.

The vaults are plain files: Markdown notes, YAML frontmatter, and JSON
manifests. teambrain only does the few things files and git cannot do
safely on their own — scaffolding capabilities, distributing them into code
repositories with ownership tracking, and promoting notes from the personal
vault to the team vault with a link-integrity gate.

Uninstall teambrain and you still have two Obsidian vaults, two git repos,
and plain .claude/ skills. Nothing is locked in.`

// flagToKey maps a persistent flag name to its configuration key.
var flagToKey = map[string]string{
	"json":     "json",
	"dry-run":  "dry_run",
	"yes":      "yes",
	"verbose":  "verbose",
	"quiet":    "quiet",
	"no-color": "no_color",
}

// newRootCommand builds the root command and the *App threaded through it.
func newRootCommand(stdio IO, build BuildInfo) (*cobra.Command, *App) {
	app := &App{IO: stdio, Build: build}

	root := &cobra.Command{
		Use:           "teambrain",
		Short:         "A personal brain and a team brain, as plain files Claude Code can share.",
		Long:          rootLong,
		Version:       build.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			v := NewViper()
			if err := bindChangedFlags(v, cmd); err != nil {
				return exit.Userf("bind flags: %v", err)
			}
			cfg, err := LoadConfig(v)
			if err != nil {
				return err
			}
			app.Config = cfg
			app.Viper = v
			app.Command = commandID(cmd)
			return nil
		},
	}

	root.SetVersionTemplate("teambrain {{.Version}}\n")
	root.SetIn(stdio.In)
	root.SetOut(stdio.Out)
	root.SetErr(stdio.Err)

	pf := root.PersistentFlags()
	pf.Bool("json", false, "emit machine-readable JSON")
	pf.Bool("dry-run", false, "show what would change without writing")
	pf.Bool("yes", false, "assume yes; skip confirmation prompts")
	pf.BoolP("verbose", "v", false, "verbose logging")
	pf.Bool("quiet", false, "suppress non-essential output")
	pf.Bool("no-color", false, "disable colored output")

	root.AddCommand(newInitCommand())
	root.AddCommand(newTeamCommand())
	root.AddCommand(newSkillCommand())
	root.AddCommand(newAgentCommand())
	root.AddCommand(newHookCommand())
	root.AddCommand(newCommandCommand())
	root.AddCommand(newCreateSyncCommand())
	root.AddCommand(newViewSyncCommand())
	root.AddCommand(newCommitSyncCommand())
	root.AddCommand(newDoctorCommand())

	root.SetContext(withApp(context.Background(), app))
	return root, app
}

// NewRootCommand exposes the root command for documentation and shell-completion
// generation. Most callers use Execute.
func NewRootCommand(stdio IO, build BuildInfo) *cobra.Command {
	cmd, _ := newRootCommand(stdio, build)
	return cmd
}

// Execute runs teambrain and returns the process exit code. It never panics on
// ordinary errors; instead it renders them per the UX contract and maps them to
// a stable code.
func Execute(stdio IO, build BuildInfo, args []string) int {
	root, app := newRootCommand(stdio, build)
	root.SetArgs(args)
	err := root.Execute()
	return renderResult(stdio, root, app, err)
}

// renderResult prints a terminal error (if any) and returns the exit code.
func renderResult(stdio IO, root *cobra.Command, app *App, err error) int {
	if err == nil {
		return int(exit.OK)
	}

	jsonMode := false
	switch {
	case app != nil && app.Config != nil:
		jsonMode = app.Config.JSON
	case root != nil:
		jsonMode, _ = root.PersistentFlags().GetBool("json")
	}

	command := ""
	if app != nil {
		command = app.Command
	}

	if jsonMode {
		_ = WriteJSON(stdio.Out, ErrorEnvelope(command, err))
		return int(exit.CodeOf(err))
	}

	fmt.Fprintf(stdio.Err, "Error: %s\n", err.Error())
	var te *exit.Error
	if errors.As(err, &te) && te.Hint != "" {
		fmt.Fprintf(stdio.Err, "Hint: %s\n", te.Hint)
	}
	return int(exit.CodeOf(err))
}

// bindChangedFlags binds only the flags the user actually set, so unset flags do
// not clobber environment or config values.
func bindChangedFlags(v *viper.Viper, cmd *cobra.Command) error {
	var firstErr error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Changed {
			return
		}
		key, ok := flagToKey[f.Name]
		if !ok {
			return
		}
		if err := v.BindPFlag(key, f); err != nil && firstErr == nil {
			firstErr = err
		}
	})
	return firstErr
}

// commandID renders a dotted command identifier for the JSON envelope, e.g.
// "skill.list". The root command yields "".
func commandID(cmd *cobra.Command) string {
	parts := strings.Fields(cmd.CommandPath())
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], ".")
}
