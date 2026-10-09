# Verification

Run the user-installed Go toolchain with automatic toolchain acquisition disabled.
`make check` checks formatting, runs vet and executes the complete race suite.
`make coverage` runs the race suite with `-coverpkg=./...`, reports exact covered
statements and fails unless every statement is covered. It does not exclude
packages or round partial coverage into success. (Local workspace, 2026)

```sh
GOTOOLCHAIN=local make check
GOTOOLCHAIN=local make coverage
pre-commit run --all-files
```

For uncommitted new files, also pass their paths with `pre-commit run --files`.
The all-files mode only includes tracked files. (Local workspace, 2026)

Run the gates sequentially. Concurrent quality and coverage runs exceeded the
ten-minute suite timeout during final verification. The sequential quality run
and coverage run passed with the same tests and timeout. See the [checkpoint](implementation.md)
for final evidence. (Local workspace, 2026)

## Test boundaries

| Boundary | Proof | Real-system effects |
| --- | --- | --- |
| Unit contracts | Catalog/configuration validation, JSON pointers, categories, fingerprints, compatibility and launch defaults | Synthetic files only |
| Lifecycle integration | Plan through verified install, update, reinstall, reset, uninstall, migration and profile selection for all eight adapters | Fake native installers and temporary homes |
| Transaction/recovery | Approval, locking, stale previews, shared owners, encrypted archives, cancellation, tampering and rollback | In-memory identities, no OS keychain mutation |
| Components | Local CRUD/enablement, parked data, native registration/result validation and cross-harness rejection | Fake vendor commands, no live plugins |
| Accounts | Encrypted vault, credential separation, stale approvals, paginated provider reports, rate limits and watch changes | In-memory keys and fixture HTTP clients |
| CLI/TUI | CLI status/argument handling, headless Bubble Tea editor commands, asynchronous operations and isolated PTY navigation | Private requests and synthetic state |

The tests cannot establish compatibility with every installed vendor version,
live Keychain prompts, real provider account data or real service restarts. Those
are excluded from development verification to preserve the user's machine.
Coverage of their native boundaries uses injected runners, transports and key
stores. (Local workspace, 2026)

## Acceptance gate

The requested target is **100% statement coverage across all production Go
packages**, with unit and integration proof. Read the [checkpoint](implementation.md)
for the latest measured result. A passing race suite does not satisfy that target
when `make coverage` fails. Do not label the coverage criterion complete or weaken
the gate to match a partial result. (User requirements, 2026; Local workspace, 2026)

The checkpoint also distinguishes observed regression failures from passing
contract tests and records material unverified combinations. (Local workspace, 2026)

## References

- Local workspace (2026). [Quality gate](../scripts/check-go.sh),
  [coverage gate](../scripts/coverage-go.sh), [TUI workflows](../internal/manager/ui_workflows_test.go),
  [lifecycle integration](../internal/manager/end_to_end_test.go),
  [monitoring safety](../internal/manager/monitor_safety_test.go).
- User requirements (2026), recorded in [the checkpoint](implementation.md).
