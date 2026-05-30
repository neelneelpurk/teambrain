# teambrain

> Two Obsidian vaults — a personal brain and a team brain — that Claude Code can read, extend, and share between. The brain is plain files; teambrain is a small, optional tool that does the few things files and git can't do safely on their own.

[![CI](https://github.com/neelneelpurk/teambrain/actions/workflows/ci.yml/badge.svg)](https://github.com/neelneelpurk/teambrain/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/neelneelpurk/teambrain.svg)](https://pkg.go.dev/github.com/neelneelpurk/teambrain)
[![Go Report Card](https://goreportcard.com/badge/github.com/neelneelpurk/teambrain)](https://goreportcard.com/report/github.com/neelneelpurk/teambrain)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

---

## What it is

`teambrain` manages two plain-text knowledge vaults for use with [Claude Code](https://claude.com/claude-code):

- a **personal brain** — private, yours alone;
- a **team brain** — shared with your team.

Both are normal [Obsidian](https://obsidian.md) vaults and normal git repositories. teambrain only adds the handful of operations that the filesystem and git cannot do safely by hand:

1. **Authoring** Claude Code capabilities (skills, agents, hooks, slash commands) with correct placement and valid frontmatter.
2. **Distributing** those capabilities into code repositories — copying the files **and** merging a hook into `settings.json` without clobbering foreign content, with ownership and checksums tracked so they can be cleanly removed.
3. **Promoting** notes from your personal brain to the team brain — an explicit copy gated by an enforced **link-integrity check** so you never publish a note with links that dangle in the team vault. The seeded `promote-to-team` skill drives the flow from inside Claude Code (deciding what to share, resolving dangling links); the deterministic commands do the staging and git.

Everything else is just files, `git`, and Obsidian.

## Why "file over app"

teambrain follows Steph Ango's (kepano's) *file over app* philosophy and Obsidian's principles of **privacy, longevity, extensibility**:

- **Everything is plain text** in open formats: Markdown notes, YAML frontmatter, plain-JSON `.teambrain.json` manifests. No opaque state, no database.
- **Files are the source of truth.** `list` is a live filesystem scan — there is no cache to desync.
- **The tool is optional.** Uninstall teambrain and you still have two Obsidian vaults, two git repos, and plain `.claude/` capabilities. Nothing is locked in. *That is the test every feature must pass.*
- **Deterministic core, agent for reasoning.** teambrain does file surgery; Claude Code does the thinking, expressed as embedded skills.
- **Degrade gracefully.** No Obsidian CLI? Use the filesystem. No agent? Deterministic output.

## Install

```sh
# With Go 1.25+
go install github.com/neelneelpurk/teambrain/cmd/teambrain@latest

# Or build from source
git clone https://github.com/neelneelpurk/teambrain
cd teambrain
make build      # produces ./bin/teambrain
```

Pre-built binaries for Linux, macOS, and Windows are attached to each [release](https://github.com/neelneelpurk/teambrain/releases).

## Quickstart

```sh
# 1. Scaffold your two vaults.
teambrain init ~/personal-brain
teambrain team init ~/team-brain

# 2. Make the team vault a git repo (your team shares this).
git -C ~/team-brain init && git -C ~/team-brain add -A && git -C ~/team-brain commit -m "init"

# 3. Bind your personal brain to one or more named team brains (1:n).
cd ~/personal-brain
teambrain team bind ~/team-eng --name eng
teambrain team bind git@github.com:acme/team-design.git --name design   # clone locally to promote
teambrain team status

# 4. Author a capability in your brain, with Claude Code's help.
teambrain skill new daily-review --description "Summarize today's notes and surface open loops"

# 5. Distribute a capability into a code repo (copies files + merges settings.json).
cd ~/code/my-service
teambrain hook import format-on-save --source ~/personal-brain   # shows the script, asks to confirm

# 6. Promote notes to the team brains. Tag a note in its frontmatter
#    (teambrains: [eng, design]) and ask Claude Code to "promote it to the team"
#    — the seeded promote-to-team skill walks the flow. Or run the primitives:
cd ~/personal-brain
teambrain create-sync                 # stage tagged notes (scans the vault, or pass paths)
teambrain view-sync                   # per-team diff AND link-integrity report
teambrain commit-sync --push          # commit ONLY those files to each team
#   commit-sync confirms before writing to the shared repo (--yes to skip) and
#   refuses a note whose links would dangle in the team vault (--force to override)
```

See the **[User Guide](USERGUIDE.md)** for the full walkthrough.

## Batteries-included skill library

teambrain ships a curated set of high-signal engineering skills **embedded in the binary** — no other LLM API required. Claude Code is the reasoning layer; these skills are the prompts that direct it, and teambrain is the deterministic harness that places and distributes them.

```sh
teambrain skill catalog        # see what's embedded
teambrain skill add code-review # install one into the current repo's .claude
```

`init` seeds the whole library into a new vault, so a fresh brain is immediately useful. The current library:

| Skill | What it does |
|---|---|
| `code-review` | Review a diff/PR for correctness, security, and clarity |
| `write-tests` | Write focused, fast, test-first tests |
| `debug` | Reproduce → isolate → root-cause → guard |
| `write-adr` | Capture an architecture decision durably |
| `write-runbook` | Write an on-call-grade operational runbook |
| `synthesize-notes` | Distill notes into decisions, actions, open questions |
| `plan-feature` | Turn a problem into a small, phased, verifiable plan |

For a team, the **team brain** is the source of truth for blessed skills: curate them there, and members `teambrain skill import <name> --source <team-brain>` into their repos. Standardization without a service to run.

## Architecture

```
~/personal-brain/             # Vault 1 — private (its own git repo, or no remote)
├── CLAUDE.md
├── .claude/                  # skills/ agents/ hooks/ commands/  + .teambrain.json (ownership)
├── _sync/                    # staging for promotion to the team (gitignored)
├── inbox/ daily/ projects/ areas/ resources/
└── .teambrain.json           # vault role + named team bindings (1:n)

~/team-brain/                 # Vault 2 — shared (its own git repo)
├── CLAUDE.md
├── .claude/                  # skills/ agents/ hooks/ commands/  + .teambrain.json
└── adrs/ design-docs/ runbooks/ conventions/ mocs/
```

Plain vaults, plain git repos — no submodules, no symlinks. Links don't cross vaults; promotion is an explicit copy. That boundary is a feature: it keeps the separation clean and removes dangling cross-vault links entirely.

**One personal brain, many team brains (1:n).** Each team is bound under a name. A note declares its destinations in its own frontmatter — `teambrains: [eng, design]` — so a single note can be promoted to several team brains at once. `create-sync` reads that property and stages the note (same relative path, or a `teambrain_dest:` override) into each target; `commit-sync` commits it into each team's git repo.

## Command reference

| Command | Purpose |
|---|---|
| `teambrain init [--here\|<path>]` | Scaffold (or repair) a personal-brain vault |
| `teambrain team init <path>` | Scaffold a team-brain vault |
| `teambrain team bind <path\|remote> [--name <n>] [--force]` | Bind a named team vault (1:n; bind several) |
| `teambrain team unbind <name>` | Remove a team binding |
| `teambrain team status` | List bound teams and their git state |
| `teambrain {skill,agent,hook,command} new <name>` | Author a new capability |
| `teambrain {skill,agent,hook,command} list` | List capabilities (live filesystem scan) |
| `teambrain skill catalog` | List the skills embedded in the binary |
| `teambrain skill add <name>` | Install an embedded library skill (no source vault or LLM needed) |
| `teambrain {skill,agent,hook,command} import <name> --source <vault>` | Copy a capability into this repo's `.claude` |
| `teambrain {skill,agent,hook,command} update <name>` | Refresh an installed capability from its source |
| `teambrain {skill,agent,hook,command} uninstall <name>` | Remove a teambrain-owned capability |
| `teambrain create-sync [path]...` | Stage tagged notes for promotion (scans the vault if no paths) |
| `teambrain view-sync` | Preview each team's payload with a diff and link-integrity report |
| `teambrain commit-sync [--push] [--force] [--yes]` | Promote each note to every tagged team, committing those files (confirms first; `--force` overrides the link gate) |
| `teambrain doctor` | Report the brain-retrieval path and check for capability tamper |

**Global flags:** `--json` · `--dry-run` · `--yes` · `--verbose/-v` · `--quiet` · `--no-color`

### Exit codes

Stable and documented, so scripts can branch on them:

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | user / validation error |
| `2` | precondition not met (e.g. no team bound) |
| `3` | external failure (git, Obsidian) |

### JSON for scripting

Every command accepts `--json` and emits a stable envelope. Human output is never meant to be parsed:

```sh
teambrain --json skill list | jq '.data.capabilities[].name'
```

```json
{
  "ok": true,
  "command": "skill.list",
  "data": { "capabilities": [ { "name": "daily-review", "kind": "skill", "path": "skills/daily-review/SKILL.md" } ] }
}
```

## Safety properties

- **`settings.json` is never clobbered.** Hook registration is a typed read-modify-write that preserves every foreign key and unknown field; only your entry is added.
- **Hooks run code, so import shows the script and confirms** (`--yes` to bypass) and never auto-runs it. `doctor` flags checksum drift (tamper detection).
- **Promotion is gated and confirmed.** `commit-sync` refuses a payload whose links would dangle in the team vault (`--force` to override) and confirms before writing to a shared repo (`--yes` to skip; required under `--json`). It stages by explicit path only — never `git add -A` — and commits only the promoted files, tolerating an otherwise-dirty tree.
- **Nothing is written outside a vault.** Path containment is enforced on every write.
- **`uninstall` is exact.** It removes only teambrain-owned files and the matching `settings.json` entry, leaving the repo byte-identical otherwise.

## Vault access

teambrain reads and writes vault files **directly on disk**, with path containment enforced on every write — nothing is ever written outside the vault it was given. There is no app-mediated backend: uninstall teambrain and the same plain files remain, openable in Obsidian, git, and Claude Code.

## Retrieval (via Obsidian)

Finding the right notes is **Obsidian's job**. Its live index, search, backlinks, and link resolver beat anything teambrain would reimplement — and they need no other LLM API. So teambrain **requires Obsidian for retrieval** (an Obsidian MCP, preferred, or the Obsidian CLI) and ships a `search-brain` skill that teaches Claude Code to use it: search first, fetch only what's needed, follow backlinks, cite `note#heading`, and never guess from filenames. `teambrain doctor` reports the active retrieval path (`obsidian-mcp` / `obsidian-cli` / `unavailable`) and `init` warns loudly if neither is present. teambrain does **not** reimplement search.

### The teambrain Obsidian MCP (`teambrain-mcp`)

teambrain ships its own Obsidian MCP server so the preferred retrieval path works out of the box. It bridges Claude Code to a **running** Obsidian vault through the [Local REST API](https://github.com/coddingtonbear/obsidian-local-rest-api) community plugin — Obsidian still does the searching — and exposes a small, **read-only**, teambrain-shaped tool set:

| Tool | Purpose |
|---|---|
| `list_vaults` | list the configured vaults (brains) and the default |
| `search_brain` | full-text search across a vault |
| `read_note` | read a note, or just one heading section |
| `read_active_note` | read the note currently open in Obsidian |
| `note_outline` | a note's heading structure |
| `list_backlinks` | notes that link to a given note |
| `list_notes` | browse the vault tree |
| `list_tags` | every tag in a vault, with counts |
| `promotion_candidates` | notes tagged `teambrains:` (feeds `promote-to-team`) |

Every tool takes an optional `vault` argument; mutations are deliberately absent — changing a vault stays the job of the deterministic CLI.

**Setup:**

1. Install and enable the **Local REST API** plugin in Obsidian; copy its API key.
2. Build it: `make build-mcp` (or `go install github.com/neelneelpurk/teambrain/cmd/teambrain-mcp@latest`).
3. Register it in Claude Code under an **obsidian**-named key (so `teambrain doctor` detects retrieval):

```json
{
  "mcpServers": {
    "obsidian-teambrain": {
      "command": "teambrain-mcp",
      "env": { "OBSIDIAN_API_KEY": "<your-key>" }
    }
  }
}
```

The single-vault path defaults to `https://127.0.0.1:27124` with the plugin's self-signed certificate; override with `OBSIDIAN_HOST`, `OBSIDIAN_PORT`, `OBSIDIAN_PROTOCOL`, `OBSIDIAN_VERIFY_TLS=true`, or `OBSIDIAN_CA_CERT=<path>` to verify against the plugin's certificate (`GET /obsidian-local-rest-api.crt`).

**Per-vault endpoints (1:n).** The Local REST API serves only the *currently-open* vault, so to reach a personal brain and several team brains at once, run an Obsidian instance per vault (each with the plugin on its own port) and point `teambrain-mcp` at a JSON config — `$TEAMBRAIN_MCP_CONFIG`, else `$XDG_CONFIG_HOME/teambrain/mcp.json`:

```json
{
  "default": "personal",
  "vaults": {
    "personal": { "api_key": "<key>", "port": 27124 },
    "eng":      { "api_key": "<key>", "port": 27125 },
    "design":   { "api_key": "<key>", "port": 27126, "ca_cert": "/path/to/design.crt" }
  }
}
```

Then tools route by name: `search_brain` with `{"vault": "eng"}` searches the eng brain; omitting `vault` uses the default. Call `list_vaults` to discover the names.

## Development

```sh
make tools      # install golangci-lint (pinned)
make ci         # fmt-check + vet + lint + race tests + coverage gate (mirrors CI)
make test       # fast unit tests
make cover-html # open an HTML coverage report
```

teambrain is built test-first (red → green → refactor) with fakes at every boundary, golden files for every generated artifact, and a thin end-to-end layer via [`testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript). See **[CONTRIBUTING.md](CONTRIBUTING.md)**.

## License

[Apache 2.0](LICENSE). See [NOTICE](NOTICE).

> The *file over app* framing follows Steph Ango's publicly stated philosophy; the application to this design is the authors', not a quote from him.
