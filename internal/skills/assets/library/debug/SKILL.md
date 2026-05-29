---
name: debug
description: Diagnose a bug methodically — reproduce, isolate, fix, and guard. Use when given an error, stack trace, failing test, or "this works in staging but not prod".
---

# Debug

Resist the urge to guess-and-patch. A bug you don't understand isn't fixed, it's
hidden.

## Search the brain for prior art first (Obsidian CLI)

Before forming hypotheses, run `obsidian --help`, then use the Obsidian CLI to
search the vault for this error, system, or symptom — prior incidents,
postmortems, and debugging notes often hold the root cause or a known fix.
Follow backlinks to the affected service's notes. Don't rediscover what a
teammate already wrote down. (No Obsidian CLI? See the `search-brain` skill.)

## Method

1. **Reproduce it deterministically.** The smallest input that triggers it. If
   you can't reproduce it, you can't know you fixed it — so make reproduction
   the first goal.
2. **Read the actual error.** The whole stack trace, the exact message. Don't
   pattern-match to a guess.
3. **Form one hypothesis at a time** and make it falsifiable. "If X is the
   cause, then Y." Then test Y — with a log, a breakpoint, a bisect.
4. **Bisect the space.** Halve it each step: which commit, which input, which
   layer. Binary search beats linear staring.
5. **Find the root cause, not the symptom.** "Why was the value nil?" → "Why
   wasn't it initialized?" Keep asking why until you reach the real defect.
6. **Fix the cause.** Then write a regression test that fails without the fix.
7. **Look for siblings.** The same mistake is often copy-pasted elsewhere.

## When it "works in staging but not prod"

The bug is in the difference: config, data shape, scale, timing, permissions,
versions. Enumerate the differences and check each.

State your current hypothesis and the next experiment out loud as you go.
