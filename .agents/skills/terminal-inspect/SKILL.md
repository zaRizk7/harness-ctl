---
name: terminal-inspect
description: Verify harness-ctl screens and keyboard flows using an isolated pseudo-terminal after TUI changes.
---

# Terminal inspect

- Run the built TUI in a pseudo-terminal with synthetic HOME, PATH, state roots
  and manager storage. Do not preview or apply changes to live harness states.
- Verify screen content and keyboard transitions. Cover the changed flow, an
  empty inventory, errors, approval and cancellation at a usable terminal size.
- Read-only navigation must leave manager storage unchanged. Mutation tests use
  fake runners and recovery keys in the Go test suite.
- Keep transcripts in scratch storage outside the repository. Stop the smoke
  process after testing. Inspect only a Terminal tab created for this task.
- Report a finding with reproduction, expected/actual behavior and severity.

Credit: distilled from https://github.com/NousResearch/hermes-agent/tree/main/skills/software-development/dogfood and https://github.com/addyosmani/agent-skills/tree/main/skills/browser-testing-with-devtools (MIT).
