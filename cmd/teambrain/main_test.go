package main

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/neelneelpurk/teambrain/internal/cli"
)

// TestMain registers an in-process "teambrain" command so testscript .txtar
// files can drive the real CLI without building a binary. The command calls
// os.Exit so that exit codes propagate to the script's exit-status assertions.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"teambrain": func() {
			os.Exit(cli.Execute(
				cli.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
				cli.BuildInfo{Version: "0.0.0-test", Commit: "testcommit", Date: "2026-05-29"},
				os.Args[1:],
			))
		},
	})
}

// TestScripts runs every testdata/script/*.txtar end-to-end against the CLI.
// The fs backend is forced so scripts are deterministic regardless of whether a
// real Obsidian CLI is installed on the host.
func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(e *testscript.Env) error {
			e.Setenv("TEAMBRAIN_VAULT_BACKEND", "fs")
			return nil
		},
	})
}
