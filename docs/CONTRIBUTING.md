# Contributing

Use macOS and the Go version declared in `go.mod`. The application supports
Apple Silicon and Intel. Runtime entry points live in `cmd/harness-ctl`, private
implementation packages in `internal`, and operator/developer guides in `docs`.
Read [AGENTS.md](../AGENTS.md), [architecture](architecture.md), the
[request ledger](requests.md) and [limitations](limitations.md) before changing
an acceptance condition. (Local workspace, 2026)

## Development workflow

```sh
git clone https://github.com/zaRizk7/harness-ctl.git
cd harness-ctl
export GOTOOLCHAIN=local
make hooks
make build
make check
make coverage
pre-commit run --all-files
```

Install `pre-commit` separately if it is unavailable. `make hooks` installs the
project's pre-commit, merge, message and push checks. Run the quality and coverage
gates sequentially. Coverage must reach every production Go statement, without
rounding or exclusions. CI verifies both macOS architectures and runs the exact
coverage gate on Apple Silicon. (Local workspace, 2026)

1. **A1 Scope.** Describe the concrete behavior and its owning module. Preserve
   approval, path ownership, shared-owner selection, stale-preview rejection,
   locking, encrypted recovery and cancellation.
2. **A2 Regression.** Follow the repository's test-first skill for behavior changes.
   Use temporary synthetic homes, fake runners/transports and in-memory keys.
   Never use development tests to change live harnesses, accounts or services.
3. **A3 Contracts.** Document Go arguments/results and trust boundaries. Update
   the relevant guide, Wiki source and request evidence with behavior changes.
   Keep documents under `docs`, apart from the README, licenses and directives.
4. **A4 Review.** Run all applicable checks and submit focused commits. Use
   conventional titles such as `fix(setup): reject changed launchers`. Commit
   bodies must be empty and co-author trailers must be absent. The hooks enforce
   these rules. Include proof and remaining limits in the pull request.

(Local workspace, 2026)

Wiki pages are versioned in [docs/wiki](wiki). Update those sources through the
same review path. Maintainers publish them as described in [releasing](releasing.md).
Do not put credentials, private state or generated binaries into an issue or PR.
(Local workspace, 2026)

## References

- Local workspace (2026). [Architecture](architecture.md), [verification](testing.md),
  [hooks](../.pre-commit-config.yaml), [CI](../.github/workflows/ci.yml),
  [test-first skill](../.agents/skills/test-first/SKILL.md).
