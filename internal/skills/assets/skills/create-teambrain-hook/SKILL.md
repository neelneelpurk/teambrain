---
name: create-teambrain-hook
description: Scaffold a new Claude Code hook in this vault's .claude/hooks directory, registering it in settings.json deterministically with `teambrain hook new`. Use when the user asks to create, author, or add a hook or automated action.
---

# Create a teambrain hook

Use this skill when the user wants a hook that runs on a Claude Code event
(for example `PreToolUse`, `PostToolUse`, `Stop`). teambrain writes the hook
script, performs a typed read-modify-write merge into `settings.json` (never
reformatting foreign content), and records ownership in `.claude/.teambrain.json`.

## Steps

1. Clarify with the user: the hook's name (kebab-case), the event it fires on,
   and exactly what the script should do.
2. Run the deterministic scaffolder:

   ```sh
   teambrain hook new <name> --event <Event>
   ```

   This writes `.claude/hooks/<name>.sh`, merges the registration into
   `settings.json` (preserving any existing hooks), and records the hook in the
   ownership manifest with a checksum.
3. Edit the generated script to do the real work. Keep it fast and side-effect
   aware — hooks run on every matching event.
4. Verify with `teambrain hook list --json`, and run `teambrain doctor` to
   confirm the checksum matches (tamper detection).

## Safety

- Hooks execute code. Review every line before relying on it.
- teambrain never auto-runs a hook on import; it shows the script and confirms.
- Removing a hook with `teambrain hook uninstall <name>` cleanly unmerges it
  from `settings.json` and the manifest.
