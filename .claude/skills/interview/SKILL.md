---
name: interview
description: Resolve material open decisions before writing a goal spec or acting on an ambiguous request.
---
# Interview

Turn a loose request into a goal spec. Ask only about material unresolved decisions. Honor decisions and authorization already given. The user decides intent, you find facts and resolve routine implementation choices.

## Rounds
- Name the destination first: the one outcome the goal delivers. It fixes the scope; push everything beyond it out of scope.
- Treat open decisions as a tree. Ask only the frontier: questions whose prerequisites are already settled. Dependencies first, one branch at a time; breadth before depth.
- Use the host's question tool when available. Keep each round short, with 2-4 options per question and the recommended option first.
- Read the repo, docs and tests before asking. If the code already decides a question, state it and move on.
- Challenge assumptions that conflict with the code or with each other; say why, offer the alternative.
- Never answer for the user. A question you cannot yet phrase sharply is fog: revisit it after the next round.

## Stop
Stop when every acceptance criterion maps to a test (`file::name`) or a justified manual check, and no material decision is open. Summarise and proceed within the user's authorization.

## Record
Write decisions into the goal file (objective, acceptance, out of scope), never only in conversation. Re-read it before the next round.

Credit: distilled from mattpocock/skills (MIT): `grill-me` https://github.com/mattpocock/skills/blob/main/skills/productivity/grill-me/SKILL.md, `grilling` https://github.com/mattpocock/skills/blob/main/skills/productivity/grilling/SKILL.md, `wayfinder` https://github.com/mattpocock/skills/blob/main/skills/engineering/wayfinder/SKILL.md
