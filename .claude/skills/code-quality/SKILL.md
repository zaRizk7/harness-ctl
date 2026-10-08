---
name: code-quality
description: Engineering standard for any code change (minimal, simple, configurable, documented). Use for every implementation, test or review task.
---

# Code quality

## Project first
- Read AGENTS.md and the project's relevant contribution instructions first. Where they differ from this standard, the project wins.
- Run the project's quality checks (tests, type checks, linters, validators) before reporting; fix failures, never skip or weaken a check.

## Standard
- Minimal footprint: smallest change that meets ACCEPT. No speculative features, abstractions, or config.
- Simple control flow, modular small files, reuse before writing, stdlib before dependencies.
- No unrelated refactors or formatting churn.
- No hard-coded configurables: any value a user or project might change (thresholds, limits, paths, names, patterns, timeouts, models) is read from config with a documented default; only protocol constants stay in code, named.
- Well-documented, standard code: follow the language's standard style and idioms; doc-comment every exported function, type and CLI; comments explain why (intent, constraints, edge cases), never restate the code, never leave commented-out code.
- No stale code or artifacts: delete what your change makes obsolete (code, scripts, docs); leave no temp or generated files.
- Reuse native harness capabilities; never force a universal execution loop.
