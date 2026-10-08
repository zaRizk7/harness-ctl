---
name: diagnose-bugs
description: Disciplined bug diagnosis. Use when a failure's cause is unknown, before changing code.
---

# Diagnose bugs
1. Reproduce: build a fast, deterministic repro that asserts the exact symptom, not just "it fails".
2. Minimise: shrink inputs and setup until nothing removable is left.
3. Hypothesise: write 3-5 falsifiable hypotheses, ranked by likelihood, each with the observation that would refute it.
4. Probe: change one variable per probe. Tag temporary logs (for example `DBG-<id>`) and remove them all afterwards.
5. Fix the cause, not the symptom, then confirm the repro passes.
6. Add a regression test at the right seam: the lowest level where the bug is observable.

Credit: distilled from https://github.com/mattpocock/skills/tree/main/skills/engineering/diagnosing-bugs and hermes-agent systematic-debugging (https://github.com/NousResearch/hermes-agent) (MIT).
