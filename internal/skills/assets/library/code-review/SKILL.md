---
name: code-review
description: Review a diff or pull request for correctness, security, and clarity before merge. Use when the user asks to review code, check a PR, or asks "is this safe to merge".
---

# Code review

Review the change like an owner who will maintain it, not a gate to pass. Be
concrete and kind. Quote the exact lines.

## Method

1. **Understand the intent first.** What problem does this change solve? If you
   can't tell from the diff and description, say so — unclear intent is the
   finding.
2. **Read the diff twice.** First for what it does; second for what it forgets.
3. **Check, in priority order:**
   - **Correctness** — edge cases, error paths, off-by-one, nil/empty, concurrency.
   - **Security** — untrusted input, injection, authz checks, secrets, path traversal.
   - **Data & migrations** — backwards compatibility, idempotency, rollback.
   - **Tests** — does a test fail without this change? Are the new paths covered?
   - **Clarity** — names, dead code, comments that explain *why*.
4. **Separate severity:** `blocking` (must fix), `should` (fix soon), `nit`
   (optional). Label every comment.

## Output

- A one-line verdict: approve / approve-with-nits / request-changes.
- Findings grouped by severity, each with file:line and a suggested fix.
- Call out what's *good* too — reviews are also how standards spread.

Prefer fewer, higher-confidence findings over a long list of maybes.
