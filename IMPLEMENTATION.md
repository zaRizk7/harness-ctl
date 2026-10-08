# Implementation checkpoint

## Accepted behavior

- macOS Apple Silicon and Intel, Go and Bubble Tea.
- Repository and executable name: harness-ctl.
- Eight supported harnesses: Codex CLI, Claude Code, Gemini CLI, OpenCode, Pi, Hermes Agent, OpenClaw, Prime Agent.
- Both existing installations and isolated managed installations, plus explicit migration.
- Install, uninstall, reinstall, reset, update/upgrade with preservation presets and category overrides.
- Protect shared resources by default. Enforced shared reset requires all affected owners.
- Shared-source controls use native capabilities or labelled shim-scoped launch profiles.
- Encrypted seven-day recovery, explicit permanent discard, service coordination, previews and verified results.
- User installs Go. Always use GOTOOLCHAIN=local, including hook commands.
- No mutations to actual harness installations or user states during implementation verification.

## Directive source

Project-local instructions, skills, roles and hook policies come from
https://github.com/zaRizk7/claude-coding-directives at
dff6aa85dc2644bba4ea71b6c6d421bc7a260f74.

The project adapts these instructions for Codex: repository skills live under
`.agents/skills/`, portable role briefs under `.agents/roles/`, and `AGENTS.md`
routes the macOS TUI workflow. Upstream hook policies remain enforced.

## Component management acceptance

The user confirmed item-level management on 2026-10-08. Preservation toggles and
profile exclusions alone do not satisfy these criteria.

| Code | Acceptance | Evidence |
| --- | --- | --- |
| C1 | Dedicated TUI categories list individual skills, MCP registrations, plugins, connectors, proxies and hooks | Named-entry inventory, category/navigation tests and isolated PTY |
| C2 | Add/install and edit concrete local resources or native registrations | Field/asset CRUD, Claude/Gemini native contracts, editor validation and PTY add preview |
| C3 | Enable/disable individual entries without losing their configuration, and remove selected entries | Skill import/restore, native flags, array restoration/removal and disabled-file editing |
| C4 | Component changes use approved previews, shared-owner selection, stale checks, encrypted recovery and rollback | Stale/changed-plan rejection, shared Codex skills, recovery, native rollback, cancellation and profile scope |
| C5 | Native capabilities, local file operations and account-managed connections are labelled accurately | Native/local labels, scoped warnings and operator capability table |
| C6 | User controls, code ownership and test evidence are documented | README, operator guide, architecture and this checkpoint |

Dedicated management now uses approved item-level operations. Native commands
are limited to verified Claude plugin and Gemini extension contracts. Local
registration/file edits do not provision remote accounts or prove vendor loader
support. (Local workspace, 2026)

Component verification on 2026-10-08 observed test-first failures for parked
array edit/removal, structured asset validation, shared user skills, native
source arguments/result checks, preview labels, disabled-directory browsing,
scope relocation, external native payloads and cancellation. An editor
fingerprint mutation exposed concurrent disabled-state changes and was reverted.
All focused regressions then passed. `make check` passed formatting, vet and
race tests. The isolated PTY passed categories, named entries, affected owners,
disable/add previews, the typed-approval guard, external editing, cancellation
and clean exit with zero manager writes. (Local workspace, 2026)

The final all-files pre-commit run passed the directive hooks and Go formatting,
vet and race tests. The feature remains verified with synthetic state and fake
native commands. Live vendor execution is excluded from this evidence.
(Local workspace, 2026)

## Current status

- Repository and executable are named `harness-ctl`.
- CLI, TUI, adapter recipes, execution lock, stale-preview rejection, journals,
  encrypted authenticated recovery and preservation controls are implemented.
- Managed installs stage binaries and use separate user-state roots. Migration
  retains the original install. Profiles use explicitly scoped shims and HOME.
- Shared deletion and shared recovery require individual affected-owner selection.
- npm tarballs are verified against the previewed integrity before installation.
- Local Go formatting, vet and race checks passed. Upstream hooks and the Go
  quality hook passed. Apple Silicon and Intel macOS binaries built successfully.
- Test-first failures were observed for execution, locking, UI error retention,
  empty-map preservation, macOS path aliases, OpenCode data-root auth, permanent
  auth erasure and missing recovery indexes. A Prime installer contract mutation
  was detected by its test and reverted.
- A blocked permanent operation retains existing recovery. Older snapshots are
  purged only after running-client checks and successful rollback capture.
- Cancellation stops an installer's complete process group before rollback.
  A synthetic child-write regression failed before this guard and passed after it.
- Homebrew ownership requires an adapter's explicit harness package binding.
  Shared Python/Node runtime packages cannot be claimed from a CLI path alone.
- Service ownership requires a matching approved label and launch executable.
  Regression tests rejected environment, working-directory, later-argument,
  nested and duplicate-key references after failing against the prior validator.
- Managed update/reinstall with an owned LaunchAgent and tracked Hermes
  update/reinstall are blocked before commands until native rebinding is verified.
  Both blockers have test-first regressions with synthetic installations.
- A real pseudo-terminal smoke test passed home/action/recovery navigation and
  keyboard exit. Read-only startup/navigation wrote no manager state.
- A prior Apple Silicon binary was launched in Terminal. Its live action
  screen was inspected in the specifically created Terminal tab.
- Verification uses synthetic installations, isolated temporary homes, fake
  runners and in-memory recovery keys. No live harness lifecycle commands were
  executed by implementation verification.
- Go remains user-managed. Dependency downloads used `GOTOOLCHAIN=local`.

## Material limits

- Component screens manage local registrations/assets. Remote authorization,
  account provisioning, OS secrets and external installer side effects remain
  native. Parked disabled state is plaintext under private directories, while
  recovery snapshots remain encrypted. See `docs/components.md`.
- Native Claude plugins whose ledger points outside selected captured state are
  rejected before execution. Copied native payloads may require reinstalling
  through the vendor in the selected scope. Gemini native commands require a
  HOME containing the selected `.gemini` directory.
- Live vendor installers, real Keychain prompts and real service restarts have
  not been exercised. Tests cover synthetic adapter and transaction contracts.
- Managed update/reinstall requires removing owned LaunchAgent registrations
  separately. Tracked Hermes update/reinstall requires its native updater or
  migration to a managed installation. These combinations do not execute.
- Profile exclusions apply to copied file state and manager shims. Project/system
  sources, OS credentials and inherited API credentials retain native behavior.
- Opaque, JSONC, dotenv and database resources use whole-resource categories.
  Custom paths and classifications require review of the concrete preview.
- Homebrew owns binary recovery. State snapshots cannot export OS credentials.
- Retention is enforced before the next lifecycle mutation. Interrupted permanent
  discard retains its encrypted rollback snapshot until recovery.

## Ownership and execution invariants

- Discovery reads executable and package metadata, not credential values or histories.
- Plan construction never writes resources. Execution requires an approved plan.
- Revalidate plan fingerprints after acquiring the single mutation lock.
- Do not delete shared, symlinked, unmanaged, or unclassified resources through a broad recursive reset.
- Validate all snapshot members before restoration. Encrypt before writing snapshot payloads.
- Preserve runtime and dependency ownership. Do not remove shared dependencies.
- Treat installed harness capabilities as adapter-specific. Report unsupported combinations explicitly.

## References

- Local workspace (2026). [Component tests](internal/manager/components_test.go),
  [TUI tests](internal/manager/tui_test.go), [operator guide](docs/components.md),
  [architecture](docs/architecture.md), [agent instructions](AGENTS.md).
- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
