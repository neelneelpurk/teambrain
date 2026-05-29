package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/scaffold"
	"github.com/neelneelpurk/teambrain/internal/team"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

func newTeamCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Create, bind, and inspect the team vault (1:1)",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newTeamInitCommand(), newTeamBindCommand(), newTeamStatusCommand())
	return cmd
}

func newTeamInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <path>",
		Short: "Scaffold a team-brain vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			v, err := vault.NewFSDirect(args[0])
			if err != nil {
				return exit.Userf("resolve team path %q: %v", args[0], err)
			}
			res, err := scaffold.TeamVault(v, app.Config.DryRun)
			if err != nil {
				return exit.Externalf("scaffold team vault: %v", err)
			}
			return app.Emit("team.init", res, func(w io.Writer) {
				verb := "created"
				if app.Config.DryRun {
					verb = "would create"
				}
				fmt.Fprintf(w, "team-brain vault at %s\n", res.Root)
				fmt.Fprintf(w, "%s %d file(s); %d already present\n", verb, len(res.Created), len(res.Existing))
			})
		},
	}
	return cmd
}

func newTeamBindCommand() *cobra.Command {
	var vaultFlag string
	var force bool
	cmd := &cobra.Command{
		Use:   "bind <path|remote>",
		Short: "Bind this personal vault to its one team vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			personalPath, err := resolvePersonalVault(app, vaultFlag)
			if err != nil {
				return err
			}
			root, err := manifest.LoadRoot(personalPath)
			if err != nil {
				if os.IsNotExist(err) {
					return exit.Preconditionf("%s is not a teambrain vault", personalPath).
						WithHint("run `teambrain init` there first")
				}
				return err
			}
			if err := team.Bind(root, args[0], time.Now().UTC().Format(time.RFC3339), force); err != nil {
				return err
			}
			if !app.Config.DryRun {
				if err := manifest.SaveRoot(personalPath, root); err != nil {
					return err
				}
			}
			return app.Emit("team.bind", root.Team, func(w io.Writer) {
				fmt.Fprintf(w, "bound team vault: %s\n", team.Describe(root.Team))
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	cmd.Flags().BoolVar(&force, "force", false, "rebind even if a different team is already bound")
	return cmd
}

func newTeamStatusCommand() *cobra.Command {
	var vaultFlag string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report the team binding and the team vault's git state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())
			personalPath, err := resolvePersonalVault(app, vaultFlag)
			if err != nil {
				return err
			}
			root, err := manifest.LoadRoot(personalPath)
			if err != nil {
				if os.IsNotExist(err) {
					return exit.Preconditionf("%s is not a teambrain vault", personalPath).
						WithHint("run `teambrain init` there first")
				}
				return err
			}

			status := map[string]any{
				"vault": personalPath,
				"bound": root.IsBound(),
			}
			if root.IsBound() {
				teamInfo := map[string]any{
					"path":     root.Team.Path,
					"remote":   root.Team.Remote,
					"bound_at": root.Team.BoundAt,
				}
				if root.Team.Path != "" {
					_, statErr := os.Stat(root.Team.Path)
					teamInfo["exists"] = statErr == nil
					teamInfo["is_git_repo"] = git.NewShell().IsRepo(root.Team.Path)
				}
				status["team"] = teamInfo
			}

			return app.Emit("team.status", status, func(w io.Writer) {
				if !root.IsBound() {
					fmt.Fprintln(w, "no team vault bound")
					fmt.Fprintln(w, "bind one with `teambrain team bind <path|remote>`")
					return
				}
				fmt.Fprintf(w, "team: %s\n", team.Describe(root.Team))
				if root.Team.Path != "" {
					_, statErr := os.Stat(root.Team.Path)
					fmt.Fprintf(w, "exists: %t\n", statErr == nil)
					fmt.Fprintf(w, "git repo: %t\n", git.NewShell().IsRepo(root.Team.Path))
				}
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	return cmd
}
