# Code architecture

The command delegates to one manager engine. Bubble Tea owns interaction and
renders previews. The engine owns approval, resource ownership, execution and
recovery. Adapters supply native contracts instead of a universal harness loop.
(Local workspace, 2026)

| Layer | Code | Responsibility |
| --- | --- | --- |
| Entry and configuration | `cmd/harness-ctl/main.go`, `internal/manager/cli.go`, `types.go` | Parse options, validate storage, use the installed Go policy |
| Adapter contracts | `catalog.go`, `discovery.go`, `plan.go` | Native state roots, verified install identity, lifecycle recipes |
| State inventory | `state.go`, `safety.go` | Classify files/fields, identify owners, reject linked paths, fingerprint previews |
| Component management | `components.go`, `components_native.go` | Item inventory, pointer/asset changes, disabled records, native plugin contracts and result verification |
| Transaction | `execution.go`, `services.go` | Serialize mutations, coordinate owned services, snapshot, execute, verify and roll back |
| Recovery | `snapshot.go`, `recovery.go` | Keychain identity, authenticated age archives, validated restoration and retention |
| Launch profiles | `profiles.go`, `registry.go` | Copy scoped state, preserve profile writes and generate managed shims |
| User interface | `tui.go`, `components_tui.go` | Screen transitions, external editor validation, owner/scope controls, typed approval and cancellation |
| Package transport | `http.go`, `package.go` | Bounded downloads and previewed package integrity |

File names in the table refer to `internal/manager/` unless an absolute repository
location is shown. Tests live beside their owning code and use synthetic homes,
fake native commands and in-memory recovery keys. (Local workspace, 2026)

## Transaction contract

1. Construct a read-only plan containing concrete resources, commands, affected
   owners, fingerprints and blockers. Selecting a screen does not approve it.
2. Require the displayed plan's identity, acquire the mutation lock, reject
   pending journals and recheck fingerprints.
3. Coordinate only owned services and reject running affected clients. Capture
   encrypted recovery before native commands or state replacement.
4. Apply only approved changes, verify the resulting state and publish registry
   changes. Preserve unrelated registrations and externally owned dependencies.
5. On failure or cancellation, stop installer children before rollback. Restore
   snapshots and services. Retain the recovery journal when restoration fails.

The flow is implemented in `buildPlan`, `validatePlan` and `execute`. Permanent
discard may irreversibly delete older recovery after rollback capture, so its
preview and approval differ from recoverable changes. (Local workspace, 2026)

## State and scope

An installation prefix owns runtime payloads. A state root owns user settings and
local resources. A launch profile owns a separate state copy used by its shim.
Managed and tracked installations share the transaction engine, but their native
recipes and recovery limits differ. (Local workspace, 2026)

Structured configurations use category-labelled JSON pointers. JSON, TOML and
YAML edits preserve unrelated values. Opaque resources remain whole files or
directories. Symlinks, shared clients and runtime payloads require explicit
ownership treatment. Category classification alone is not a vendor capability
contract or permission to execute an installer. (Local workspace, 2026)

Component requests use the `manage` transaction action. Planning produces exact
writes or native commands and a digest binding the approved request to its
sources, owners and destinations. JSON pointers address map entries, array items
and scalar settings. Disabled array records retain original positions, rebased
after removal. Native enablement uses only documented flags/contracts. Other
entries move into category-labelled parked storage, which remains local
plaintext state. Recovery archives remain encrypted. (Local workspace, 2026)

Codex user skills follow HOME separately from CODEX_HOME. Inventory includes
only `.agents/skills` and its parked skill records, with shared ownership for
base launches and private profile HOME ownership. Component editors check the
actual active or parked source before and after editing. Temporary files are
bounded and removed on every normal result path. Cancellable copying finishes
before rollback. See the [operator guide](components.md) for supported native
adapters and external-side-effect limits. (Local workspace, 2026)

## Agent documentation

`AGENTS.md` routes project work. `.agents/skills/` contains discoverable Codex
skills, including the TUI-specific `terminal-inspect` workflow. `.agents/roles/`
contains optional role briefs read by an explicitly requested coordinator.
The briefs do not configure models, grant tools or launch workers. Codex discovers
repository skills under `.agents/skills/`. (OpenAI, 2026)

The skills and Git hook policies retain the upstream directive provenance.
Adaptation removes Claude-specific dispatch assumptions and keeps the same
privacy, evidence, test-first and no-co-author policies. (zaRizk7, 2026)

## References

- Local workspace (2026). [Engine](../internal/manager/execution.go),
  [state](../internal/manager/state.go), [planning](../internal/manager/plan.go),
  [recovery](../internal/manager/snapshot.go).
- OpenAI (2026). [Repository skills](https://learn.chatgpt.com/docs/build-skills).
- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
