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
- A real pseudo-terminal smoke test passed home/action/recovery navigation and
  keyboard exit. Read-only startup/navigation wrote no manager state.
- The latest Apple Silicon binary was launched in Terminal. Its live action
  screen was inspected in the specifically created Terminal tab.
- Verification uses synthetic installations, isolated temporary homes, fake
  runners and in-memory recovery keys. No live harness lifecycle commands were
  executed by implementation verification.
- Go remains user-managed. Dependency downloads used `GOTOOLCHAIN=local`.

## Material limits

- Live vendor installers, real Keychain prompts and real service restarts have
  not been exercised. Tests cover synthetic adapter and transaction contracts.
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
