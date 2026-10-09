# Development

The Go module uses `cmd/harness-ctl` for the executable, functionality-owned
private packages in `internal`, automation in `scripts` and guides in `docs`.
The [architecture guide](https://github.com/zaRizk7/harness-ctl/blob/main/docs/architecture.md)
maps responsibilities and transaction invariants. Implementation directives stay
model/harness agnostic. Native role definitions inherit the active model and are
used only for explicitly requested delegation. (Project, 2026)

```sh
export GOTOOLCHAIN=local
make hooks
make build
make check
make coverage
pre-commit run --all-files
```

Install Go and `pre-commit` yourself before running these commands. Gates run
sequentially. Formatting, vet and race tests are mandatory. The strict coverage
gate counts every production Go statement and fails on even one uncovered
statement. It does not establish all-path correctness or shell coverage.
Tests use synthetic homes, fake native commands, fixture transports and in-memory
keys rather than changing a contributor's real harnesses or provider accounts.
(Project, 2026)

Behavior changes follow the test-first directive. Document arguments/results,
update the operator guide and durable request evidence, and use conventional
atomic commits with empty bodies and no co-author trailers. See
[contribution guidance](https://github.com/zaRizk7/harness-ctl/blob/main/docs/CONTRIBUTING.md)
for the review path. (Project, 2026)

CI checks both macOS architectures and produces commit-versioned build artifacts.
A future version tag triggers verified binaries and a **draft** release. No
binary release is currently published. The
[maintainer guide](https://github.com/zaRizk7/harness-ctl/blob/main/docs/releasing.md)
describes release review and Wiki publication. (Project, 2026)

Wiki sources live in
[docs/wiki](https://github.com/zaRizk7/harness-ctl/tree/main/docs/wiki).
Propose tutorial edits there so they remain reviewable with the code, then
publish them to the separate Wiki repository without force-pushing.
(Project, 2026)

## References

- Project (2026). [Contribution guidance](https://github.com/zaRizk7/harness-ctl/blob/main/docs/CONTRIBUTING.md),
  [verification](https://github.com/zaRizk7/harness-ctl/blob/main/docs/testing.md),
  [maintainer guide](https://github.com/zaRizk7/harness-ctl/blob/main/docs/releasing.md),
  [AGENTS.md](https://github.com/zaRizk7/harness-ctl/blob/main/AGENTS.md).
