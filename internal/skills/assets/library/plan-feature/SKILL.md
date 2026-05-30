---
name: plan-feature
description: Turn a problem statement or feature idea into a small, phased plan with clear acceptance criteria. Use when scoping work, breaking down a vague ask, or before starting implementation.
---

# Plan a feature

A plan's job is to make the work small, ordered, and verifiable — and to surface
the risky unknowns before they cost a week. Bias toward the smallest version
that delivers value.

## Find prior art and constraints first

Use the **search-brain** skill to search the vault for related projects, prior
ADRs, and constraints. Plan against what the team already knows, not a blank
page — search-brain reads the live vault through Obsidian (MCP preferred, else
the CLI) rather than guessing from filenames.

## Method

1. **Restate the problem and the user.** One sentence. If you can't, stop and
   clarify — a plan for the wrong problem is waste.
2. **Define done.** Write the acceptance criteria first, as checkable
   statements. The plan exists to satisfy these.
3. **Find the spine.** The thinnest end-to-end slice that a user could actually
   use. Everything else is a later phase.
4. **Phase it.** Each phase ships something, is independently verifiable, and
   leaves the system working. Order by dependency and by risk-retired-per-step.
5. **Name the unknowns.** What might invalidate the plan? Put the riskiest
   assumption in phase one so you learn early and cheaply.
6. **List non-goals.** What you are deliberately *not* doing. This is half of
   good scoping.

## Output

```markdown
## Problem
## Acceptance criteria
- [ ] ...
## Phases
1. <slice> — verifiable by <check>
## Risks / unknowns
## Non-goals
```

Prefer three honest phases over a ten-step plan that pretends to know the future.
