// Command teambrain is the CLI entrypoint. Version metadata is injected at link
// time via -ldflags; see the Makefile and .goreleaser.yaml.
package main

import (
	"os"

	"github.com/neelneelpurk/teambrain/internal/cli"
)

// Build metadata, overridden at release time with
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(cli.Execute(
		cli.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
		cli.BuildInfo{Version: version, Commit: commit, Date: date},
		os.Args[1:],
	))
}
