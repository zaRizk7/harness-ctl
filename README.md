# harness-ctl

A macOS TUI for installing, updating, reinstalling, resetting, uninstalling and
migrating coding harnesses. It manages Codex CLI, Claude Code, Gemini CLI,
OpenCode, Pi, Hermes Agent, OpenClaw and Prime Agent. Other detected harnesses
are listed as unsupported.

## Build and run

Install Go yourself. This repository uses Go 1.27.1 and explicitly disables
automatic toolchain acquisition.

```sh
make build
./bin/harness-ctl
```

`make run` builds and opens the TUI. `./bin/harness-ctl --help` shows global
options. The compiled binary does not require Go at runtime.

Useful read-only commands:

```sh
./bin/harness-ctl catalog
./bin/harness-ctl list
./bin/harness-ctl config
./bin/harness-ctl --root '/absolute/manager-storage' tui
```

Startup reads executable locations, installation manifests and known service
metadata. It does not invoke harness commands or open authentication/history
files. State inventory is read when you request an operation preview.

## Controls

| Screen | Controls |
| --- | --- |
| Harness list | Up/Down, Enter select, `d` refresh, `r` recovery, `q` quit |
| Actions | Tab cycles existing installations, Enter selects action |
| Preservation | Space toggles, `v` edits target version, Enter previews |
| Preview | Up/Down scroll, type `apply` and Enter, Esc cancels |
| Permanent discard | Type `discard` and Enter |
| Launch profile | Space selects exclusions, Enter previews, type `profile` |
| Profile base launch | `b` returns to base state and retains profile files |
| Recovery | Enter previews restore, Space selects affected owners, type `restore` |
| Interrupted operation | Type `recover` to restore its journaled snapshot |
| Snapshot deletion | `x`, then type `purge` and Enter |
| Running operation | Ctrl+C cancels and waits for rollback |

No lifecycle operation runs from merely selecting a harness or opening a preview.

## Installation and state ownership

New installations use manager-owned prefixes. Existing installations remain
tracked through their verified npm, Homebrew or native installation method.
Migration copies state into a managed location and retains the old installation.
Uninstall the old installation separately after checking the new launch.

Managed prefixes and user state are separate. Updates stage a new prefix before
publishing its shim. Managed npm packages must match the previewed package name
and version. Downloaded package bytes must match the previewed SHA-512 or SHA-256
integrity before npm receives them. Dependencies and lifecycle scripts retain
their native installers and permissions.

The manager creates shims under its configured `bin_dir`. To use one explicitly,
run the absolute shim path shown by `config`, or add that directory to PATH
yourself. The manager does not edit shell startup files.

| Harness | Managed user-state convention |
| --- | --- |
| Codex CLI | `CODEX_HOME` |
| Claude Code | `CLAUDE_CONFIG_DIR` |
| Gemini CLI | Private shim HOME containing `.gemini` |
| OpenCode | Private XDG config, data, state and cache directories |
| Pi | `PI_CODING_AGENT_DIR` |
| Hermes Agent | `HERMES_HOME`, native pinned runtime and exact-install launcher |
| OpenClaw | `OPENCLAW_STATE_DIR` |
| Prime Agent | `PRIME_AGENT_CODING_AGENT_DIR` |

Choose preserve-all, auth-only or discard-all, then override individual categories.
Factory reset starts with discard-all. Other lifecycle actions start with
preserve-all. Reset preserves the executable and dependencies. Uninstall removes
the selected executable and retains categories you marked to preserve.

JSON, TOML and YAML configurations support nested selective edits. Preserved
settings are verified by value. Opaque files, JSONC, dotenv files and databases
are handled as whole resources in their displayed category. Classification uses
known configuration fields and filename conventions. Review the exact resource
list, especially custom layouts and unclassified `other` state.

Shared files stay intact unless every listed affected owner is selected. Linked
sources and user workspaces are protected. Runtime checkouts, dependency stores
and project files have separate ownership and are excluded from generic reset.
Known LaunchAgents are stopped only after their plist proves ownership by the
selected installation. Previously loaded services are resumed, except after a
successful uninstall. A running affected client blocks mutation.

## Launch source profiles

A profile makes a persistent copy of a managed installation's user state and
omits selected file categories and links. Its shim uses a private HOME. Proxy
exclusion also unsets the usual HTTP/HTTPS/ALL/NO proxy environment variables.
Rebuilding a profile replaces profile-written state after taking encrypted
recovery. Updates carry profile-written state to the new installation.

These controls apply to manager shims. Original launchers, project/system
sources, OS credentials and inherited API credentials retain their native
behavior. A profile is a separate state copy, not a sandbox or a continuously
synchronized view. Native global disable controls are not invented where the
adapter has no verified contract.

## Recovery

Before mutation, the manager encrypts recovery with age and authenticates the
ciphertext with a separate HMAC. The private key stays in macOS Keychain under
`harness-ctl.snapshot-key`. Payloads are encrypted before being written to disk.
The display index exposes paths and operation metadata, never state contents.

The default retention is seven days, enforced before the next lifecycle
mutation. Restore validates and stages the full archive before replacing live
paths. Shared recovery requires affected-owner selection. Restoring one harness
retains other harness registrations.

Permanent discard deletes matching older recovery archives. A temporary
encrypted rollback snapshot exists during execution, then is erased after
completion or successful rollback. An interrupted operation retains it until
recovery. This deletes manager copies and native local credentials selected in
the preview. It does not revoke remote sessions, remove externally managed
backups or promise physical secure erasure on APFS/SSD storage.

The operation lock prevents simultaneous mutations. Interrupted journals block
new operations until recovered. Keychain failure blocks mutation rather than
falling back to plaintext. Homebrew binary rollback requires its package manager.
State snapshots cannot export OS-held credentials. Native logout therefore
requires permanent discard and affected-owner selection.

## Configuration

Supply a partial JSON object with `--config /absolute/config.json`. Omitted
fields retain defaults. `--root` overrides storage and its bin directory.

| Field | Default |
| --- | --- |
| `home` | Current user's home |
| `root` | `~/Library/Application Support/harness-ctl` |
| `bin_dir` | `<root>/bin` |
| `backup_days` | `7` |
| `probe_seconds` | `8`, read-only recipe lookup timeout |
| `operation_seconds` | `1800` |
| `max_snapshot_bytes` | `4294967296`, file payload limit |
| `metadata_bytes` | `8388608` |
| `installer_bytes` | `4194304` |
| `package_bytes` | `536870912` |
| `release_channel` | `stable`, only supported channel |
| `state_roots` | `{}`, optional tracked-state overrides by harness ID |

Use absolute paths. Root, home and state ownership boundaries are validated.
Managed state paths are derived from root and cannot be redirected by a registry.
For a custom root in a JSON config, set `bin_dir` to a child of the same root,
or use `--root` to derive it automatically.

## Verification and development

```sh
make hooks
make check
pre-commit run --all-files
```

The installed pre-commit, pre-merge-commit, commit-msg and pre-push hooks retain
the upstream directive checks and add Go formatting, vet and race tests. Tests
use temporary synthetic homes, fake command runners and in-memory snapshot keys.
They never run lifecycle commands against live user installations.

Vendor installer commands have been checked against the consulted sources and
synthetic adapter contracts. Live vendor installations, network downloads,
Keychain UI prompts and real service restarts require verification on disposable
machines before treating every vendor version as validated.

## References

- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives), pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
- Local workspace (2026). [Manager source](internal/manager), [lifecycle tests](internal/manager/lifecycle_test.go), [recovery tests](internal/manager/snapshot_test.go), [TUI tests](internal/manager/tui_test.go).
- Nous Research (2026). [Installation](https://hermes-agent.nousresearch.com/docs/getting-started/installation), [launchers](https://github.com/NousResearch/hermes-agent/blob/main/hermes_cli/_launchers.py).
- Prime Intellect (2026). [State-directory implementation](https://github.com/PrimeIntellect-ai/prime-agent/blob/main/crates/pa-types/src/platform/dirs.rs), [native installer](https://app.primeintellect.ai/prime-agent/install.sh).
- OpenAI (2026). [Native installer](https://chatgpt.com/codex/install.sh).
- Google (2026). [Gemini storage](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/config/storage.ts).
