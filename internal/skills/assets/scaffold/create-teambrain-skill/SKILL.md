---
name: create-teambrain-skill
description: Scaffold a new Claude Code skill in this vault's .claude/skills directory and register it deterministically with `teambrain skill new`. Use when the user asks to create, author, or add a new skill to their brain.
---

# Create a teambrain skill

Use this skill when the user wants to author a new Claude Code skill inside this
vault. teambrain handles deterministic placement and valid frontmatter; you
supply the reasoning and the body.

## Steps

1. Clarify with the user: the skill's name (kebab-case), its single-sentence
   purpose, when it should trigger, and the concrete steps it performs.
2. Run the deterministic scaffolder:

   ```sh
   teambrain skill new <name> --description "<one-line description>"
   ```

   This creates `.claude/skills/<name>/SKILL.md` with valid frontmatter and a
   stub body, and it is discoverable immediately — the file *is* the
   registration.
3. Open the generated `SKILL.md` and replace the stub body with clear, concise
   instructions for the agent. Keep the description specific: it is what future
   sessions match against.
4. Verify with `teambrain skill list --json`.

## Principles

- One skill, one job. Prefer small, composable skills.
- The description is a trigger, not a title — make it concrete.
- Never hand-edit frontmatter `name`; let `teambrain skill new` own placement.
