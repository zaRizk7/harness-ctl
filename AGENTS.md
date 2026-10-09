# Repository instructions for harness-ctl

## Scope and ownership

- Read the accepted behavior and checkpoint in `docs/implementation.md`, then relevant
  code, callers and tests. Implement the user's requested behavior and stop after
  verification. Report missing acceptance criteria instead of claiming completion.
- Extend the layer that owns the invariant. Harness contracts belong in adapters,
  state ownership in inventory/planning, and rollback in the transaction engine.
- Use synthetic homes, fake runners and in-memory recovery keys for tests. Never
  run lifecycle or component changes against the user's real harness state during
  development verification. Go is user-managed. Always set `GOTOOLCHAIN=local`.
- Access task-relevant project files, development configuration, tool metadata
  and public documentation. Do not read unrelated personal files, credentials,
  histories or repositories. Do not log component configuration values.
- Preserve explicit approval, shared-owner selection, path validation, stale
  preview checks, locking, encrypted recovery and cancellation guarantees.

## Skills and roles

- Repository skills live in `.agents/skills/`. Load `code-quality` before code
  changes and `test-first` for behavior changes. Select other skills by their
  descriptions and load only relevant supporting resources.
- Project-scoped native subagent configurations live in `.codex/agents/`.
  They inherit the current model unless the user requests an override. Load
  named skills explicitly and respect the host's permissions and isolation.
- Use `orchestrator` only when the user explicitly requests orchestration.
  Otherwise work directly. Moving documentation does not authorize delegation.
- Agent instructions stay model and harness agnostic. Preserve the pinned
  directive policies and hook enforcement, update references, and keep one source
  per rule. Repository edits never change global agent configuration.

## Documentation and verification

- `README.md` is the project summary. Operator and developer documents live in
  `docs/`. Licenses and this primary directive remain at the repository root.
  `docs/architecture.md` explains code ownership and transaction invariants.
  `docs/implementation.md` is the acceptance record and resumable checkpoint.
- Document Go packages, types and functions with standard comments explaining
  arguments, results and non-obvious trust boundaries. When behavior changes,
  update the relevant user guide and its acceptance evidence in the same change.
- Run `make check` and the configured Git hooks. Install missing hooks with
  `make hooks`. Never weaken or bypass a check. Keep commits atomic and omit
  co-author trailers.
- Report exact commands, concise results, relevant file/line references and
  material unverified behavior. Re-read the checkpoint after compaction.

## References

- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  adapted from `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
- OpenAI (2026). [Repository instructions](https://learn.chatgpt.com/docs/agent-configuration/agents-md),
  [repository skills](https://learn.chatgpt.com/docs/build-skills).
