package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/capability"
)

// storeFor builds a capability.Store for the .claude directory under dir,
// honoring --dry-run.
func storeFor(app *App, dir string) *capability.Store {
	return capability.OpenStore(filepath.Join(dir, ".claude")).WithDryRun(app.Config.DryRun)
}

// newSkillCommand assembles `teambrain skill list|new`.
func newSkillCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "skill", Short: "Author and list skills", Args: cobra.NoArgs}
	cmd.AddCommand(newListCommand("skill", capability.KindSkill))

	var dir, description string
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new skill in .claude/skills",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			res, err := storeFor(app, dir).NewSkill(args[0], description)
			if err != nil {
				return err
			}
			return emitNew(app, res)
		},
	}
	newCmd.Flags().StringVar(&dir, "dir", ".", "directory containing the .claude folder")
	newCmd.Flags().StringVar(&description, "description", "", "one-line description (the trigger)")
	cmd.AddCommand(newCmd)
	cmd.AddCommand(newImportCommand("skill", capability.KindSkill))
	cmd.AddCommand(newUpdateCommand("skill", capability.KindSkill))
	cmd.AddCommand(newUninstallCommand("skill", capability.KindSkill))
	return cmd
}

// newAgentCommand assembles `teambrain agent list|new`.
func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Author and list agents", Args: cobra.NoArgs}
	cmd.AddCommand(newListCommand("agent", capability.KindAgent))

	var dir, description string
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new agent in .claude/agents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			res, err := storeFor(app, dir).NewAgent(args[0], description)
			if err != nil {
				return err
			}
			return emitNew(app, res)
		},
	}
	newCmd.Flags().StringVar(&dir, "dir", ".", "directory containing the .claude folder")
	newCmd.Flags().StringVar(&description, "description", "", "one-line description")
	cmd.AddCommand(newCmd)
	cmd.AddCommand(newImportCommand("agent", capability.KindAgent))
	cmd.AddCommand(newUpdateCommand("agent", capability.KindAgent))
	cmd.AddCommand(newUninstallCommand("agent", capability.KindAgent))
	return cmd
}

// newHookCommand assembles `teambrain hook list|new`.
func newHookCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "hook", Short: "Author and list hooks", Args: cobra.NoArgs}
	cmd.AddCommand(newListCommand("hook", capability.KindHook))

	var dir, event, matcher string
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a hook script and register it in settings.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			res, err := storeFor(app, dir).NewHook(capability.HookOptions{
				Name:    args[0],
				Event:   event,
				Matcher: matcher,
			})
			if err != nil {
				return err
			}
			return emitNew(app, res)
		},
	}
	newCmd.Flags().StringVar(&dir, "dir", ".", "directory containing the .claude folder")
	newCmd.Flags().StringVar(&event, "event", "", "Claude Code hook event (e.g. PostToolUse)")
	newCmd.Flags().StringVar(&matcher, "matcher", "", "tool matcher for tool-scoped events")
	cmd.AddCommand(newCmd)
	cmd.AddCommand(newImportCommand("hook", capability.KindHook))
	cmd.AddCommand(newUpdateCommand("hook", capability.KindHook))
	cmd.AddCommand(newUninstallCommand("hook", capability.KindHook))
	return cmd
}

// newCommandCommand assembles `teambrain command list|new|import|update|uninstall`.
func newCommandCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "command", Short: "Author and list slash commands", Args: cobra.NoArgs}
	cmd.AddCommand(newListCommand("command", capability.KindCommand))

	var dir, description string
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new slash command in .claude/commands",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			res, err := storeFor(app, dir).NewCommand(args[0], description)
			if err != nil {
				return err
			}
			return emitNew(app, res)
		},
	}
	newCmd.Flags().StringVar(&dir, "dir", ".", "directory containing the .claude folder")
	newCmd.Flags().StringVar(&description, "description", "", "one-line description")
	cmd.AddCommand(newCmd)
	cmd.AddCommand(newImportCommand("command", capability.KindCommand))
	cmd.AddCommand(newUpdateCommand("command", capability.KindCommand))
	cmd.AddCommand(newUninstallCommand("command", capability.KindCommand))
	return cmd
}

// newListCommand builds a `list` subcommand that prints capabilities of one kind.
func newListCommand(noun string, kind capability.Kind) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %ss discovered in .claude (live filesystem scan)", noun),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())
			all, err := storeFor(app, dir).List()
			if err != nil {
				return err
			}
			filtered := make([]capability.ListItem, 0, len(all))
			for _, it := range all {
				if it.Kind == string(kind) {
					filtered = append(filtered, it)
				}
			}
			return app.Emit(noun+".list", map[string]any{"capabilities": filtered}, func(w io.Writer) {
				if len(filtered) == 0 {
					fmt.Fprintf(w, "no %ss found\n", noun)
					return
				}
				for _, it := range filtered {
					if it.Event != "" {
						fmt.Fprintf(w, "%-24s %s  [%s]\n", it.Name, it.Path, it.Event)
					} else {
						fmt.Fprintf(w, "%-24s %s\n", it.Name, it.Path)
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing the .claude folder")
	return cmd
}

// emitNew reports an authoring result.
func emitNew(app *App, res *capability.NewResult) error {
	return app.Emit(res.Kind+".new", res, func(w io.Writer) {
		verb := "created"
		if app.Config.DryRun {
			verb = "would create"
		}
		fmt.Fprintf(w, "%s %s %q\n", verb, res.Kind, res.Name)
		for _, f := range res.Created {
			fmt.Fprintf(w, "  + %s\n", f)
		}
	})
}
