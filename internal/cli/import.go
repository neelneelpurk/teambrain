package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/capability"
	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/manifest"
)

// newImportCommand builds `teambrain <noun> import <name>`.
func newImportCommand(noun string, kind capability.Kind) *cobra.Command {
	var dir, from, mode string
	var sources []string

	cmd := &cobra.Command{
		Use:   "import <name>",
		Short: fmt.Sprintf("Copy %s %s from a vault into this repo's .claude", article(noun), noun),
		Long: fmt.Sprintf(`Copy %s %s authored in a vault into this repo's .claude, recording ownership so
it can later be updated or cleanly removed.

Sources come from --source (repeatable) or the personal_vault set in config (its
bound team vaults are searched too). If the name exists in more than one source,
pass --from <label> to choose. Hooks run code, so importing one shows the script
and asks first (--yes to skip).`, article(noun), noun),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			srcs, err := resolveSources(app, sources)
			if err != nil {
				return err
			}
			store := storeFor(app, dir)

			plan, err := store.PlanImport(capability.ImportOptions{
				Name:    args[0],
				Kind:    kind,
				Sources: srcs,
				From:    from,
				Mode:    mode,
			})
			if err != nil {
				return err
			}

			// Hooks run code: show the script and confirm before applying.
			if kind == capability.KindHook && !app.Config.Yes {
				ok, err := confirmHook(app, plan)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(app.IO.Err, "aborted; nothing was imported")
					return nil
				}
			}

			res, err := store.Apply(plan)
			if err != nil {
				return err
			}
			return app.Emit(noun+".import", res, func(w io.Writer) {
				verb := "imported"
				if app.Config.DryRun {
					verb = "would import"
				}
				fmt.Fprintf(w, "%s %s %q from %q (%s)\n", verb, res.Kind, res.Name, res.Source, res.Mode)
				for _, f := range res.Files {
					fmt.Fprintf(w, "  + %s\n", f)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "target directory containing .claude")
	cmd.Flags().StringArrayVar(&sources, "source", nil, "candidate source vault path (repeatable)")
	cmd.Flags().StringVar(&from, "from", "", "disambiguate by source label")
	cmd.Flags().StringVar(&mode, "mode", "copy", "copy (snapshot) or link (symlink that tracks the source; skipped on Windows)")
	return cmd
}

// newUpdateCommand builds `teambrain <noun> update <name>`.
func newUpdateCommand(noun string, _ capability.Kind) *cobra.Command {
	var dir string
	var sources []string
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: fmt.Sprintf("Refresh an installed %s from its source vault", noun),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			srcs, err := resolveSources(app, sources)
			if err != nil {
				return err
			}
			res, err := storeFor(app, dir).Update(args[0], srcs)
			if err != nil {
				return err
			}
			return app.Emit(noun+".update", res, func(w io.Writer) {
				if res.Changed {
					fmt.Fprintf(w, "updated %q (%s → %s)\n", res.Name, short(res.OldChecksum), short(res.NewChecksum))
				} else {
					fmt.Fprintf(w, "%q already up to date\n", res.Name)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "target directory containing .claude")
	cmd.Flags().StringArrayVar(&sources, "source", nil, "candidate source vault path (repeatable)")
	return cmd
}

// newUninstallCommand builds `teambrain <noun> uninstall <name>`.
func newUninstallCommand(noun string, _ capability.Kind) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: fmt.Sprintf("Remove a teambrain-owned %s from this repo", noun),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			if !app.Config.Yes && !app.Config.DryRun {
				ok, err := confirm(app, fmt.Sprintf("Uninstall %s %q (removes owned files and its settings entry)?", noun, args[0]))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(app.IO.Err, "aborted")
					return nil
				}
			}
			res, err := storeFor(app, dir).Uninstall(args[0])
			if err != nil {
				return err
			}
			return app.Emit(noun+".uninstall", res, func(w io.Writer) {
				fmt.Fprintf(w, "uninstalled %q\n", res.Name)
				for _, f := range res.Removed {
					fmt.Fprintf(w, "  - %s\n", f)
				}
				for _, f := range res.Missing {
					fmt.Fprintf(w, "  ? %s (already gone)\n", f)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "target directory containing .claude")
	return cmd
}

// resolveSources assembles candidate source vaults from --source flags and, if
// configured, the personal vault (and its bound team vault).
func resolveSources(app *App, sourceFlags []string) ([]capability.Source, error) {
	var sources []capability.Source
	seen := map[string]bool{}

	add := func(label, vaultPath string) {
		claudeDir := claudeDirOf(vaultPath)
		if seen[claudeDir] {
			return
		}
		seen[claudeDir] = true
		sources = append(sources, capability.Source{Label: label, ClaudeDir: claudeDir})
	}

	if pv := app.Config.PersonalVault; pv != "" {
		add("personal", pv)
		if root, err := manifest.LoadRoot(pv); err == nil {
			for _, b := range root.Teams {
				if b.Path != "" {
					add(b.Name, b.Path)
				}
			}
		}
	}
	for _, src := range sourceFlags {
		add(filepath.Base(strings.TrimSuffix(filepath.Clean(src), string(filepath.Separator))), src)
	}

	if len(sources) == 0 {
		return nil, exit.Userf("no import sources").
			WithHint("pass --source <vault> or set personal_vault in config")
	}
	return sources, nil
}

// claudeDirOf returns the .claude directory for a vault path, accepting either a
// vault root or a .claude directory.
func claudeDirOf(p string) string {
	if filepath.Base(p) == ".claude" {
		return p
	}
	return filepath.Join(p, ".claude")
}

func confirmHook(app *App, plan *capability.Plan) (bool, error) {
	fmt.Fprintf(app.IO.Err, "About to import hook %q from %q.\n", plan.Name, plan.SourceLabel)
	fmt.Fprintf(app.IO.Err, "Event: %s\n", plan.HookEvent)
	fmt.Fprintln(app.IO.Err, "This script will run on that event. Review it:")
	fmt.Fprintln(app.IO.Err, "----------------------------------------")
	fmt.Fprintln(app.IO.Err, plan.ScriptPreview)
	fmt.Fprintln(app.IO.Err, "----------------------------------------")
	return confirm(app, "Import this hook?")
}

func confirm(app *App, prompt string) (bool, error) {
	fmt.Fprintf(app.IO.Err, "%s [y/N]: ", prompt)
	reader := bufio.NewReader(app.IO.In)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

// article returns the indefinite article for noun ("an" before a vowel).
func article(noun string) string {
	if noun != "" && strings.ContainsRune("aeiou", rune(noun[0])) {
		return "an"
	}
	return "a"
}

func short(checksum string) string {
	const n = 14
	if len(checksum) <= n {
		return checksum
	}
	return checksum[:n] + "…"
}
