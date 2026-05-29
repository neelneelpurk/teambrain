---
name: create-teambrain-agent
description: Scaffold a new Claude Code subagent in this vault's .claude/agents directory and register it deterministically with `teambrain agent new`. Use when the user asks to create, author, or add a new agent to their brain.
---

# Create a teambrain agent

Use this skill when the user wants to author a new Claude Code subagent inside
this vault. teambrain places the file and writes valid frontmatter; you define
the agent's role and system prompt.

## Steps

1. Clarify with the user: the agent's name (kebab-case), the kind of task it
   owns, the tools it should have, and the model (if it matters).
2. Run the deterministic scaffolder:

   ```sh
   teambrain agent new <name> --description "<one-line description>"
   ```

   This creates `.claude/agents/<name>.md` with valid frontmatter and a stub
   system prompt.
3. Edit the generated file: write a focused system prompt describing the agent's
   responsibilities, constraints, and output format.
4. Verify with `teambrain agent list --json`.

## Principles

- An agent is a specialist. Give it one clear domain, not a grab-bag.
- Describe *when* to delegate to it; that description drives routing.
