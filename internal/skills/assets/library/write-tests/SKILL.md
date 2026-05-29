---
name: write-tests
description: Write focused, fast tests for a unit of code, test-first when possible. Use when the user asks to add tests, improve coverage, or write a test for a function or bug.
---

# Write tests

A test exists to catch a regression a human would otherwise ship. Optimize for
that, not for coverage numbers.

## Method

1. **Name the behavior, not the method.** `TestWithdraw_rejectsOverdraft`, not
   `TestWithdraw1`.
2. **Test through the public surface.** Don't reach into internals; if you must,
   the design is telling you something.
3. **One reason to fail per test.** Table-driven cases for variations of the
   same behavior; separate tests for separate behaviors.
4. **Cover the boundaries:** empty, one, many; zero, negative, max; the error
   path; the concurrent path.
5. **Make failures legible.** Assert with the expected and actual values in the
   message. A failing test should tell you what broke without a debugger.
6. **Keep them fast and hermetic.** No real network, clock, or filesystem unless
   that *is* the unit under test — use a fake at the boundary.

## For a bug

Write the failing test that reproduces it *first*. Watch it fail for the right
reason. Then fix the code and watch it pass. That test is now a permanent guard.

## Verify

Run the suite. A test you never saw fail is a test you don't trust.
