---
name: test-first
description: Test-first workflow for any language. Use when implementing or changing behaviour, or fixing a bug.
---

# Test first
1. Write or extend a test for the exact behaviour wanted, in the project's own test framework.
2. Run it and see it fail for the right reason. If it cannot fail without the change, it proves nothing: fix the test, or prove it with a mutation (break the code, watch it fail, restore).
3. Make the smallest change that passes. No extra features, abstractions or unrelated edits.
4. Re-run that test, then the full suite.
5. Commit the failing test and the change separately when the history should show the order.

Evidence to report, compactly: the failing run (command + one-line failure), the passing run (command + counts).
