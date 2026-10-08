---
name: challenge-claims
description: Checklist for challenging a completion claim instead of trusting it. Use when reviewing or testing someone else's work.
---

# Claims: never trust, always challenge
Treat every completion claim as unproven. Challenge against, until each is resolved:
- acceptance criteria (each one, individually)
- tests: do they exist, fail without the change, pass with it
- diff: scope creep, missing parts, unrelated files, dead code
- evidence: exact commands + results; re-run decisive ones yourself (compact output)
- assumptions: stated, checked, or flagged
- regressions: what else could this break
- risks: unknowns, edge cases, security

Unresolved item -> send back with a precise question. Do not accept "should work".
Conflicting reports -> ask for evidence on the disputed fact, run the cheapest decisive check, decide, record why.
