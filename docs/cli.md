# CLI, batch operations and launches

Global `--config FILE` and `--root DIR` options precede the command. Command
options precede harness IDs. `--yes` approves the concrete preview printed by the
same process. Without it, enter the displayed batch ID. `--preview` writes
nothing. The CLI and TUI use the same engine. (Local workspace, 2026)

```sh
harness-ctl install --preview codex claude pi
harness-ctl install --yes codex claude pi
harness-ctl uninstall --preserve auth --yes codex claude
harness-ctl update --yes pi gemini
harness-ctl reinstall --target 1.2.3 pi
harness-ctl reset --preserve auth,skills,plugins pi
harness-ctl reset --preserve none --permanent pi
```

1. **D1 Batch safety.** One preview covers all selected harnesses. The manager
   validates every item before the first mutation and holds one lock throughout.
   Operations commit in order. Failure rolls back the failed item and stops the
   rest. Earlier successes keep their individual recovery records.
2. **D2 Preservation.** CLI defaults preserve all categories, including reset. Choose `none`, `auth`
   or comma-separated categories. Shared deletion requires `--owners` with the
   exact owners shown by the preview. Overlapping destructive batch state is
   rejected and needs one scoped operation.
3. **D3 Selection.** `--install-id ID` selects a detected installation for a
   single-harness operation. `list` prints IDs. New installations use isolated
   prefixes. Existing ownership restrictions still apply.

The home screen's `b` opens batch selection. Space selects harnesses, Enter
chooses an action, and the normal options/preview controls approve the batch.
(Local workspace, 2026)

## Components without the TUI

```sh
harness-ctl components list pi plugins
harness-ctl components apply --preview codex request.json
harness-ctl components apply --yes codex request.json
```

Use the [component request schema](components.md). Requests specify the concrete
path, field or native package and retain the same stale-preview, owner and
recovery checks. Configuration values are withheld from previews.
(Local workspace, 2026)

## Launch and PATH

```sh
harness-ctl codex --model MODEL
harness-ctl claude --model MODEL
harness-ctl launch --native codex -- --help
harness-ctl launch --install-id INSTALL_ID pi
harness-ctl launch --account work-api codex
harness-ctl path
```

Add the `path` command's printed `export PATH=...` line to your shell profile to
run managed shims as `codex`, `claude` and other native command names. Explicit `setup --shell-file ABSOLUTE_FILE` previews and approves the same PATH
activation in a POSIX startup file. The shims scope HOME/state and forward
arguments to the selected native executable. They continue to work after manager
self-removal when harness files are retained. (Local workspace, 2026)

| Harness | Shipped launch behavior |
| --- | --- |
| Codex CLI | Workspace-write sandbox, on-request approval and automatic reviewer |
| Claude Code | Native `--permission-mode auto`, subject to version/model/provider policy |
| Gemini CLI | Native `--approval-mode auto_edit`, edit approval with other tools still prompting |
| OpenCode | Native permission rules, labelled secure-auto limitation |
| Pi | Native defaults, no universal approval flag |
| Hermes | Native policy, with the missing secure auto-mode contract labelled |
| OpenClaw | Native `tui`, with native tool approval policy |
| Prime Agent | Native defaults |

The automatic reviewer uses `approvals_reviewer="auto_review"`. It reviews eligible
approval requests without removing the sandbox. Managed native policy may still
restrict it. The shipped defaults do not include unrestricted permission bypass
flags. (OpenAI, 2026; Local workspace, 2026)

Overrides replace only their matching independent default. A sandbox override
retains approval/reviewer defaults, and a model-only config override retains all
security defaults. Explicit bypass is a user override, never a shipped default.
Help and
version calls receive no added approval flags. `launch --native` or
`HARNESS_CTL_NATIVE=1` for direct shims selects native defaults. Native modes are
not interchangeable. OpenClaw/Hermes/Pi/Prime do not have a verified common auto-mode
switch in this release. Use `launch_rules` for independent options, `config_flags` for native config
assignments and `launch_note` for capability limits. Each rule has `args`,
`flags` and optional `config_keys`. Legacy `launch_args`/`approval_flags` remain
supported. Unchanged shipped legacy policies normalize in memory to independent
rules. Custom policies and installed catalog files are preserved.
(Local workspace, 2026)

Claude auto availability depends on supported models, provider/server policy and
local/managed settings. Native checks govern fallback. `launch --native` selects
the native starting policy explicitly. Gemini auto-edit is narrower automation
than approving every tool. Its sandbox follows native configuration. Secure
modes reduce approval prompts but cannot guarantee all actions are safe.
(Anthropic, 2026; Google, 2026; Local workspace, 2026) (Local workspace, 2026)

## Self-uninstall

```sh
harness-ctl self-uninstall --preview
harness-ctl self-uninstall
harness-ctl self-uninstall --harnesses
harness-ctl self-uninstall --state
harness-ctl self-uninstall --harnesses --state --preserve none
harness-ctl self-uninstall --harnesses --preserve none --owners OWNER_LIST
harness-ctl self-uninstall --harnesses --preserve none --permanent --preview
```

The TUI home screen's `u` opens the same independent choices. Harness removal
preserves user state by default. `--preserve` selects harness categories,
`--owners` selects each affected shared owner, and `--permanent` chooses harness
recovery erasure. The TUI exposes these choices when harness removal is selected,
with `o` for individual owner selection. `--state` permanently erases the manager's
registry, encrypted account/library vaults, catalog/configuration, recovery archives and temporary downloads.
Retained harness executables, their states, profiles and direct shims remain in
place beneath the storage root. A synchronization inode remains to preserve lock
safety. The recorded manager launcher and installation receipt are removed, and the running executable is removed last. The source repository
and shared runtimes are retained. Both shipped architecture-specific binary names
and `harness-ctl` support self-removal. (Local workspace, 2026)

## Catalog configuration

Explicit [setup](setup.md) initializes `<root>/catalog.json`. Startup uses this editable user-owned catalog when present. `config` prints the complete active JSON configuration. Edit its `harnesses`
array, or set `catalog_file` to a JSON array of adapter specifications. The
embedded [catalog data](../internal/catalog/catalog.json) supplies defaults,
without a hard-coded Go catalog. Each engine owns its configured catalog.
Unknown native installation kinds are rejected. (Local workspace, 2026)

Required fields are `id`, `name`, `command`, `kind` and `default_home`. The `npm`
kind also requires `package`. Optional fields include `legacy_packages`,
`brew_packages`, `home_env`, `config_files`, `shared_clients`, `launch_labels`,
`docs`, `launch_rules`, `config_flags`, `launch_args`, `launch_note` and
`approval_flags`. Native adapter code owns format and
installation contracts, so changing an ID does not create a new native adapter.
`additional_commands` configures read-only detection of unsupported commands.
(Local workspace, 2026)

## References

- Local workspace (2026). [CLI](../internal/manager/cli.go),
  [batch engine](../internal/manager/batch.go), [launch](../internal/manager/launch.go),
  [self-removal](../internal/manager/self.go), [catalog](../internal/manager/catalog.go).
- OpenAI (2026). [CLI reference](https://developers.openai.com/codex/cli/reference).
  [Configuration reference](https://developers.openai.com/codex/config-reference).
- Anthropic (2026). [CLI reference](https://code.claude.com/docs/en/cli-reference),
  [permission modes and availability](https://code.claude.com/docs/en/permission-modes).
- Google (2026). [Approval and sandbox configuration](https://geminicli.com/docs/reference/configuration/).
- OpenCode (2026). [Permissions](https://opencode.ai/docs/permissions/).
- Pi contributors (2026). [Packages](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/packages.md).
- Nous Research (2026). [Native CLI](https://github.com/NousResearch/hermes-agent/blob/main/hermes_cli/main.py).
