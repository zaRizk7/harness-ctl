---
name: goal-spec
description: Use after resolving open decisions to write or change a goal spec in the project's existing specification location.
---
# Goal spec

The goal file is the only spec: no second document. Use the project's existing specification location or the host's planning document. Write it from decisions already given. Do not interview again or invent answers. If a tracked spec needs editing, delegate that edit to a worker worktree.

## Write
- Follow the project's spec format. Include a title, objective and dependencies where relevant.
- Objective: the outcome a user or maintainer can observe, not the solution. Name modules or interfaces only where a decision was made; no file paths or code that will rot.
- State what is out of scope and which assumptions you rely on, so a worker cannot widen or guess. Ask only about unresolved decisions that materially affect the outcome.
- Put worker boundaries (always, ask first, never) in the out-of-scope lines so briefs can copy them into LIMITS.
- If one request bundles capabilities that could ship and be verified separately, split it into several goals with `depends` edges instead of one large goal.

## Acceptance
- Each criterion is observable behaviour, one line: `— test: file::name`. Prefer the highest existing test seam; add a new one only when none fits.
- Use `— manual: <steps>` only when no test is possible, and say why.
- Test what the system does, not how it does it.

## Check
Re-read as a worker with no context: can each criterion be proven true or false? Does anything need a decision still? Run the project's spec validator if it has one. Ask for confirmation only when a material decision remains unresolved, honoring authorization already given.

Credit: distilled from MIT-licensed skills: mattpocock/skills `to-spec` https://github.com/mattpocock/skills/blob/main/skills/engineering/to-spec/SKILL.md; addyosmani/agent-skills `spec-driven-development` https://github.com/addyosmani/agent-skills/blob/main/skills/spec-driven-development/SKILL.md and `idea-refine` https://github.com/addyosmani/agent-skills/blob/main/skills/idea-refine/SKILL.md (MIT)
