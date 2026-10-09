# Lifecycle and batches

Global `--config` and `--root` options go before the command. Command options go
before harness IDs. Preview first, review the actual resources and commands, then
approve the batch ID printed by the executing process. `--yes` approves that
process's preview. A previous preview is not an approval token for a new process.
(Project, 2026)

```sh
harness-ctl list
harness-ctl install --preview codex claude pi
harness-ctl install codex claude pi
harness-ctl update --preview codex pi
harness-ctl update codex pi
harness-ctl uninstall --preserve auth --preview claude pi
harness-ctl uninstall --preserve auth claude pi
```

One approval covers the selected harnesses. Execution holds one mutation lock,
revalidates every plan and applies operations sequentially. A failure rolls back
the failed item and stops later items. Earlier successes retain their individual
recovery records. Overlapping destructive shared state needs a scoped operation.
(Project, 2026)

## Preserve selected state

CLI operations preserve all categories by default, including reset. Choose
`--preserve auth`, a comma-separated category list, or `none`. TUI factory reset
starts with discard-all, so review its category controls before approval.
(Project, 2026)

```sh
harness-ctl reset --preserve auth,skills,plugins --preview pi
harness-ctl reinstall --install-id INSTALL_ID --preview pi
harness-ctl reset --preserve none --preview pi
```

Replace `INSTALL_ID` with an ID from `list`. Shared deletion requires each owner
shown by the preview, selected with `--owners` or the TUI's owner screen. Running
clients, uncertain ownership and unsupported service/updater combinations block
changes rather than weakening recovery guarantees. (Project, 2026)

## TUI and launch

Run `harness-ctl` to open the TUI. Press `b` on the home screen, select harnesses
with Space, then choose the action with Enter. Review preservation choices and
type the requested approval phrase. Unavailable actions/state controls are hidden
when no installed harness or retained state supports them. (Project, 2026)

```sh
harness-ctl codex --model MODEL
harness-ctl claude --model MODEL
harness-ctl launch --native codex -- --help
```

Replace `MODEL` with a native supported model. Managed PATH shims also accept
native command names directly. Shipped launch policies prefer verified secure
automatic modes and label native fallback limits. They do not default to
unrestricted bypass flags. See the
[launch table](https://github.com/zaRizk7/harness-ctl/blob/main/docs/cli.md#launch-and-path)
for each harness's actual policy. (Project, 2026)

## References

- Project (2026). [CLI guide](https://github.com/zaRizk7/harness-ctl/blob/main/docs/cli.md),
  [operator controls](https://github.com/zaRizk7/harness-ctl/blob/main/docs/usage.md),
  [batch engine](https://github.com/zaRizk7/harness-ctl/blob/main/internal/manager/batch.go).
