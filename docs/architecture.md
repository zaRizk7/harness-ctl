# Code architecture

The command delegates to one manager engine. Bubble Tea owns interaction and
renders previews. The engine owns approval, resource ownership, execution and
recovery. Adapters supply native contracts instead of a universal harness loop.
(Local workspace, 2026)

| Layer | Code | Responsibility |
| --- | --- | --- |
| Entry and configuration | `cmd/harness-ctl/main.go`, `internal/manager/cli.go`, `types.go` | Parse options, validate storage, use the installed Go policy |
| Adapter contracts | `internal/catalog/`, `catalog.go`, `discovery.go`, `plan.go` | Validated configurable catalog, native state roots, verified install identity, lifecycle recipes |
| State inventory | `state.go`, `internal/stateconfig/`, `internal/storage/` | Classify files/fields, identify owners, reject linked paths, fingerprint previews |
| Component management | `internal/component/`, `components.go`, `components_native.go`, `marketplaces.go`, `compatibility.go` | Item inventory, pointer/asset changes, disabled records, harness-specific plugin contracts and result verification |
| Transaction | `execution.go`, `services.go` | Serialize mutations, coordinate owned services, snapshot, execute, verify and roll back |
| Recovery | `snapshot.go`, `archive_platform.go`, `recovery.go` | Keychain identity, authenticated age archives, validated restoration and retention |
| Launch profiles | `profiles.go`, `registry.go` | Copy scoped state, preserve profile writes and generate managed shims |
| Launch execution | `launch.go`, `internal/launch/` | Secure adapter defaults, argument forwarding, interactive streams and inference-key selection |
| Batch and manager removal | `batch.go`, `self.go` | One locked sequential batch, stale previews, independent state choices and executable removal last |
| Provider accounts | `accounts.go`, `accounts_cli.go`, `internal/providers/`, `internal/appserver/`, `internal/vault/` | Encrypted account vault, separate inference/reporting keys, bounded paginated read-only reports and provider cooldowns |
| Shared library | `internal/library/`, `library.go`, `library_cli.go`, `library_tui.go` | Encrypted reusable records, compatibility, captured assets and approved sequential fan-out |
| Setup | `internal/setup/`, `setup.go`, `setup_tui.go`, `scripts/install.sh` | Publisher checksum, private prefix, optional owned launcher, non-overwriting configuration initialization |
| User interface | `internal/editor/`, `tui.go`, `components_tui.go`, `management_tui.go`, `monitor_tui.go`, `capabilities.go` | Screen transitions, private editor requests and validation, owner/scope controls, typed approval and cancellation |
| Package transport | `internal/download/`, `http.go`, `package.go` | Bounded HTTPS downloads, streaming integrity verification and atomic artifact publication |
| Native filesystem | `filesystem.go`, `internal/storage/` | Private native IO boundary, confinement, atomic storage and deterministic failure proof |
| Configuration encoding | `encoding.go`, `internal/stateconfig/` | Native JSON/TOML/YAML serialization, bounded request publication and deterministic encoding failure proof |

Package paths are repository-relative. Remaining file names in the table refer to `internal/manager/` unless an absolute repository
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

If both replacement and rollback renames fail, the original tree remains in the
replacement staging directory. The returned error identifies that retained path.
Cleanup removes the staging directory only when it no longer holds the sole
original payload. (Local workspace, 2026)

The flow is implemented in `buildPlan`, `validatePlan` and `execute`. Permanent
discard may irreversibly delete older recovery after rollback capture, so its
preview and approval differ from recoverable changes. (Local workspace, 2026)

Batches validate all plans before the first mutation and retain one lock across
ordered execution. Each item retains its own recovery record. Failure restores
the failed item and stops later ones, preserving earlier committed results.
Self-removal uses the same batch engine and removes the manager executable last.
Manager metadata deletion excludes retained harness states, profiles and shims. The recorded manager launcher is separately verified and removed.
(Local workspace, 2026)

Account mutations share the filesystem lock and encrypted storage identity.
Reporting uses a separate HTTPS client that refuses redirects and withholds
provider error bodies. Scoped engine copies share provider cooldowns through a
pointer, so no mutex is copied. Reporting loads an independent registry snapshot before launching the explicitly
selected native account protocol, avoiding transaction-owned registry/profile
state. The app-server module owns bounded initialization/quota reads and process
cleanup. Google costs use validated read-only export pages. Provider caching and
HTTP throttling preserve upstream intervals. Billing changes use native links. (Local workspace, 2026)

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

## Functional ownership

`stateconfig` owns categories, native codecs, classification and field traversal.
`component` owns local field/asset planning, disabled-array positions, native flag
recognition and manifest compatibility. It returns planned writes without owning
approval, publication, snapshots or rollback. Its IO contract keeps confinement
and failure injection at the transaction boundary. `launch` owns independent
argument defaults and their POSIX representation. Direct launches and shims use
that same policy. Remaining manager tests cover engine coordination and native
adapter transactions across these modules. (Local workspace, 2026)

`nativeauth` owns catalog-declared command matching and native argument validation.
`credentials` owns compatible profile identities and declared file locations.
Manager authentication runs the selected real process with its launch environment,
while profile capture/restore uses the existing approved transaction and encrypted
vault. Native output and credential bytes stay outside previews and journals.
OS-held secrets and remote OAuth state remain native. Read-only external component
sources never grant mutation ownership. (Local workspace, 2026)

## Agent documentation

`AGENTS.md` routes project work. `.agents/skills/` contains the engineering
workflows. `.codex/agents/` contains native project-scoped role configurations.
Descriptions and instructions are model and harness agnostic. Models inherit
from the current session. Delegation requires an explicit request. (Local workspace, 2026)

The skills and Git hook policies retain the pinned directive provenance and its
privacy, evidence, test-first and no-co-author policies. (zaRizk7, 2026)

## Go contracts and verification

Package, type and function comments describe arguments, results and the trust
boundaries at their owning layer. Read the package with `go doc ./internal/manager`
or `go doc -u ./internal/manager` for internal contracts. The [verification guide](testing.md)
records the unit/integration boundaries and exact coverage gate. (Local workspace, 2026)

## References

- Local workspace (2026). [Engine](../internal/manager/execution.go),
  [state](../internal/manager/state.go), [planning](../internal/manager/plan.go),
  [recovery](../internal/manager/snapshot.go).
- OpenAI (2026). [Repository skills](https://learn.chatgpt.com/docs/build-skills).
- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
