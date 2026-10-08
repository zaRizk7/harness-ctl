---
name: worker-protocol
description: Brief and report packet formats plus worker git discipline, evidence and parking. Use for every delegated task.
---

# Worker protocol

Follow the brief, work only in WORKDIR, and reply with the report packet.

## Brief (orchestrator -> worker), one message, terse
```
GOAL: <one sentence outcome>
TASK: <what this worker does; role>
WORKDIR: <worktree path>  (branch <project's worker branch>)
ACCEPT: <numbered, testable criteria>
CONTEXT: <only facts and paths needed; pointers, not dumps>
LIMITS: <scope, forbidden areas, budget, no merge/push, privacy boundary>
REPORT: <format below, line cap>
COMMIT: <subject>  (optional; follow the project's commit rules)
```

## Report (worker -> orchestrator), compact, no prose padding
```
STATUS: done | partial | blocked
CLAIMS: <what is now true, one line each, mapped to ACCEPT numbers>
EVIDENCE: <exact commands + one-line results; paths:lines>
DIFFSTAT: <git diff --stat summary>
RISKS: <assumptions, unverified items, regressions possible>
NEXT: <open questions, follow-ups, or park state>
```

## Rules
- Git: commit only on your own branch. Never merge, push or change the protected branch. Commit before reporting and pass the project's hooks.
- Quality: before reporting run the project's configured checks and put their one-line results in EVIDENCE.
- Evidence: treat your own claims as unproven. No report without EVIDENCE: exact commands and one-line results.
- Background tasks and monitors: stop them before reporting.
- Parking (quota or context low): commit, report STATUS partial, NEXT = the resume point.
