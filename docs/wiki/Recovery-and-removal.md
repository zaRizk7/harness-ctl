# Recovery and removal

Mutations capture encrypted authenticated recovery before changing owned local
state. Retention defaults to seven days, enforced before the next lifecycle
mutation. Keychain failure blocks changes rather than producing plaintext
recovery. Snapshots do not capture remote account changes, OS-held secrets or
arbitrary installer effects. (Project, 2026)

## Recover captured state

Press `r` on the TUI home screen. Select a snapshot, review its paths and affected
owners, then type `restore` to approve. Interrupted journals block new mutations
until recovery. Their approval phrase is `recover`. Ctrl+C during execution
cancels the operation and waits for rollback. A failed rollback retains its
journal or staging data for recovery. (Project, 2026)

## Remove the manager independently

Preview the exact manager/harness/state choices first:

```sh
harness-ctl self-uninstall --preview
harness-ctl self-uninstall
harness-ctl self-uninstall --harnesses --preview
harness-ctl self-uninstall --harnesses
harness-ctl self-uninstall --state --preview
```

The first removal retains harnesses and their state. `--harnesses` also removes
selected harness installations while preserving all harness state categories by
default. `--state` permanently removes manager configuration, catalog, account
and library vaults, recovery archives and temporary downloads. Retained harness
shims/payloads remain independently usable. The repository and shared runtimes
are retained. (Project, 2026)

Press `u` on the home screen for the equivalent TUI choices. Harness-state
selection uses `--preserve`, shared owners use `--owners`, and harness recovery
erasure uses `--permanent`. These are separate from manager-state erasure.
(Project, 2026)

```sh
harness-ctl self-uninstall --harnesses --preserve auth --preview
harness-ctl self-uninstall --harnesses --preserve none --permanent --preview
```

Permanent discard cannot revoke remote sessions or guarantee physical secure
erasure on APFS/SSD storage. Review each concrete preview and preserve recovery
until its loss is intended. Existing shared resources and uncertain ownership
remain protected. See the
[recovery contract](https://github.com/zaRizk7/harness-ctl/blob/main/docs/usage.md#recovery)
and [self-removal controls](https://github.com/zaRizk7/harness-ctl/blob/main/docs/cli.md#self-uninstall).
(Project, 2026)

## References

- Project (2026). [Recovery](https://github.com/zaRizk7/harness-ctl/blob/main/docs/usage.md),
  [self-uninstall](https://github.com/zaRizk7/harness-ctl/blob/main/docs/cli.md),
  [transaction invariants](https://github.com/zaRizk7/harness-ctl/blob/main/docs/architecture.md).
