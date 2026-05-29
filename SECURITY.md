# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities **privately**, not in a public issue.

- Use [GitHub's private vulnerability reporting](https://github.com/neelneelpurk/teambrain/security/advisories/new) ("Report a vulnerability" on the Security tab), or
- contact the maintainers through the channel listed on the project's GitHub profile.

We aim to acknowledge reports within a few days and to coordinate a fix and disclosure timeline with you.

## Supported versions

teambrain is pre-1.0. Security fixes are applied to the latest release.

## Security model & considerations

teambrain is a local CLI that manipulates files in vaults and code repositories. Its threat model centers on a few deliberate design choices:

- **Hooks execute code.** A hook is a script that Claude Code runs on an event. teambrain treats hook distribution as a supply-chain concern:
  - `hook import` **shows the script and requires confirmation** before installing (bypass only with explicit `--yes`), and never runs the hook itself.
  - Every owned capability is checksummed at install time; `teambrain doctor` flags drift so a modified hook is detected.
  - Review any hook before relying on it, especially one received from someone else.

- **`settings.json` is merged, never blindly rewritten.** Registration is a typed read-modify-write that preserves all foreign keys and unknown fields. Removal unmerges only teambrain's own entry.

- **Writes are path-contained.** teambrain refuses to read or write outside the vault it was given, and refuses promotion destinations that escape the team vault.

- **Git staging is path-scoped.** Promotion uses `git add -- <paths>` and a pathspec-limited commit, so it can never sweep unrelated working-tree changes into a commit.

- **No secrets, no network in the core.** The core makes no LLM or network calls and stores no tokens. There is nothing to leak from teambrain's own operation.

If you find a way to violate any of these properties — for example, a path that escapes containment, a `settings.json` merge that drops or corrupts foreign content, or a promotion that commits unintended files — that is a security issue worth reporting privately.
