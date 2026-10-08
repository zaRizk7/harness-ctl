---
name: simplify-safely
description: Behaviour-preserving simplification. Use when asked to simplify, clean up or refactor existing code.
---

# Simplify safely
- Behaviour is preserved exactly. Tests stay unchanged; if one must change, it is not a simplification.
- Chesterton's fence: before removing or reshaping something odd, find out why it exists (history, callers, tests).
- Follow the project's conventions. Clarity over brevity: fewer lines that are harder to read is not simpler.
- One simplification per change; run the tests after each.
- In feature work, no drive-by edits: simplify only what the task touches, and separately from the feature.

Credit: distilled from https://github.com/addyosmani/agent-skills/tree/main/skills/code-simplification (MIT).
