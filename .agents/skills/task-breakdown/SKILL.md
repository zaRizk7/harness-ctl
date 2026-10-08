---
name: task-breakdown
description: Use as orchestrator before dispatching workers, to split a goal's acceptance criteria into an ordered set of worker briefs.
---
# Task breakdown

Plan from the goal file; a task is one worker brief (format: worker-protocol). No separate ticket files or tracker.

## Slice
- Each task is a vertical slice: a thin end-to-end path (code, test, docs) that is verifiable on its own. Never one layer at a time.
- Size it for one fresh worker run: few files, one concern, one atomic commit series. Split anything bigger, and any task whose ACCEPT needs more than 3 lines or whose title needs "and".
- ACCEPT of each brief is a subset of the goal's criteria, each as `<file>::<name>` (or its `manual:` steps). Every criterion lands in at least one task.
- Do any preparatory refactor first, as its own task. A wide mechanical change goes expand, migrate in batches, then contract.

## Order
- List tasks in dependency order and state what blocks each; tasks with no shared blocker may run in parallel up to the worker limit.
- Put the riskiest unknown first. If it is unclear, spike it with an investigator (facts) or the architect (decision) before briefing implementers.

## Check
Show the user the numbered list (title, blocked by, what it delivers) when granularity or order is a real choice; otherwise dispatch. Write tasks in the conversation, not in the repo.

Credit: distilled from MIT-licensed skills: mattpocock/skills `to-tickets` https://github.com/mattpocock/skills/blob/main/skills/engineering/to-tickets/SKILL.md; addyosmani/agent-skills `planning-and-task-breakdown` https://github.com/addyosmani/agent-skills/blob/main/skills/planning-and-task-breakdown/SKILL.md (MIT)
