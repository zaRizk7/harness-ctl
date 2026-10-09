# Implementation checkpoint

The durable [user request ledger](requests.md) records every post-plan request,
accepted clarification and open audit finding. Read it before continuing work.
The historical completion statement below was superseded by the read-only audit
and the user's authorization to resolve its gaps. Current verification and open
items are recorded below.
(User, 2026; Local workspace, 2026)

## Active native-management checkpoint, 2026-10-09

Final review reproduced a retained-launcher regression after manager removal
with both manager-state choices. Native auth prefixes unconditionally executed
the removed router. The regression failed first with exit status 126. The narrow
launcher fix checks router executability and otherwise lets native authentication
reach the retained real harness with its recorded profile environment and no
session approval arguments. Ordinary sessions keep their security policy. Focused
race tests passed for routing and both removal choices. The full quality gate
passed formatting, vet and race tests, with manager tests at 413.976 seconds.
The unchanged strict race-enabled coverage gate then passed at exactly
7,972/7,972 production Go statements. The correction is candidate v0.1.1,
with all-files directive hooks passed and commit/push hooks and publication pending.
Existing v0.1.0 launchers require approved regeneration. No live state was used.
(Local workspace, 2026)

The user authorized completing the remaining gaps and a functional published
bootstrap. Native authentication routing and encrypted native credential profiles
are now explicit requirements A4–A6 and I4 in the ledger. Repository code-quality,
test-first and lean-build instructions apply. No live harness/account verification
is authorized. API keys remain manager encrypted accounts. OAuth runs through
native commands, with opt-in approved capture/restore of supported native files.
OS Keychain credentials remain native. The installer regression failed against
the old script and now passes for POSIX shell, automatic checksums/architecture,
pinned versions, invalid tags and checksum mismatch. Auth regression failed with
unknown lifecycle action, and the selected-command regression now passes.
Native CLI/TUI authentication, launcher routing and encrypted credential profile
capture/restore/removal are implemented. Focused tests prove selected installation
and profile scopes, private/encrypted storage, shared-owner approval, stale native
refresh rejection, partial-write rollback and permanent profile erasure. Explicit
read-only project/system component sources and non-user native plugin ledger
provenance are implemented. Codex marketplaces use native commands, TOML inventory,
scoped cache recovery and local-source fingerprints. The full non-race suite and
the existing full race check passed. The unchanged strict race-enabled coverage
gate passed at exactly 7,961/7,961 production Go statements. Its first run exposed
one missing legacy-auth normalization path despite rounded 100.0%, and the new
regression covers that path without changing the production source or gate.
The isolated native CLI/TUI smoke passed status routing and capture previews with
zero credential/recovery writes. Both versioned macOS architectures built. POSIX
shell, actionlint workflow, 25 Markdown/local links and 14 JSON-example checks
passed before release-note creation. The final local gates and publication
progress are recorded below. The first candidate release is v0.1.0.

Subsequent credential review found that current catalog changes prevented listing
or removing saved copies. The obsolete-profile regression failed first. Stored
schema validation is now separate from current restore compatibility, preserving
removal while refusing unsafe application. Focused race tests passed. This small
source change supersedes the 7,961-statement result for final publication. Fresh
strict race-enabled coverage now passes at exactly 7,972/7,972 production Go
statements. The sequential quality gate passed formatting, vet and the complete
race suite. The manager package took 422.290 seconds. All-files directive hooks
also passed. No gate was weakened. The binary release and actual distribution
checks are complete as recorded below.
(User, 2026; Local workspace, 2026)

The three conventional atomic commits `4a4dae1`, `e769e8c` and `51a7be1` passed
their configured hooks with empty commit bodies. The isolation script verified
restored bytes and modes before dropping each temporary stash. The final working
tree was clean. Normal pre-push hooks passed, then source `main` and the annotated
`v0.1.0` tag were published at `51a7be159f6dcc6e0e677024d1c2a84f73dc8612`.
The matching native-auth/component/setup Wiki changes were published at `54dce4c`.
Final release links and installation proof are published at `847f83a`, with all
eight Wiki files matching their versioned sources.
All 26 current Markdown files/local links and 14 JSON examples passed validation.
A local-candidate setup smoke passed direct/symlink installation, private modes,
PATH preservation, version and zero credential/recovery writes. This candidate
check does not establish published-asset installation.
[Source CI](https://github.com/zaRizk7/harness-ctl/actions/runs/37919674921) and
[release CI](https://github.com/zaRizk7/harness-ctl/actions/runs/37919675206) passed
on both architectures. The reviewed [v0.1.0 release](https://github.com/zaRizk7/harness-ctl/releases/tag/v0.1.0)
is public. Actual published-bootstrap verification passed. No live harness/account
state was inspected or mutated. (Local workspace, 2026; GitHub, 2026)

| Code | Native-management and release proof | Result |
| --- | --- | --- |
| NR1 | Local `make check`, `make coverage` and all-files hooks | Formatting, vet and full race suite passed. Exact 7,972/7,972 production Go statements. No checks changed |
| NR2 | Atomic commits and normal source/tag push | All configured commit, message and pre-push hooks passed. Empty bodies, no co-author trailers, verified restoration, clean tree and no history rewrite |
| NR3 | Source and release CI at `51a7be1` | Both macOS architectures passed native quality/race checks, versioned builds and uploads. Release arm64 independently covered exactly 7,972/7,972 statements and printed `harness-ctl v0.1.0` |
| NR4 | Draft download, GitHub digests and local setup | All four assets match GitHub digests. Both binary architectures match their names. Installer bytes match the tag. Direct/symlink setup passed in disposable homes |
| NR5 | Actual public `curl ... \| sh` on Apple Silicon | Latest and pinned installer/binary routes passed. Headless direct/symlink and piped interactive setup with a controlling TTY, typed approval and clean exit passed |
| NR6 | Published setup ownership and storage | Binary mode 0700, catalog/config mode 0600, symlink target, home-confined prefix, retained shell bytes, explicit PATH append, five-second configuration and version passed. Zero account/credential/recovery writes |

(Local workspace, 2026; GitHub, 2026)

The first public interactive smoke reached successful setup but timed out while
its Python runner stopped draining terminal output during shutdown. The runner
now drains the PTY while waiting. The same published binary and installer passed
the subsequent complete smoke. No production Go or installer change was needed.
Intel's published binary has native CI quality/build/version proof, but actual
public-bootstrap installation was exercised locally only on Apple Silicon.
Signing/notarization and the remaining consumer-reporting/native adapter gaps
remain in [limitations](limitations.md). A passing release does not close those
requirements. Final publication notes are documentation-only and preserve the
verified production bytes. (Local workspace, 2026; GitHub, 2026)

## Publication checkpoint, 2026-10-09

The user authorized `gh` publication of the public repository
`zaRizk7/harness-ctl`, a quality Go application structure and Wiki tutorials.
This supersedes every historical deferred-publication instruction below.
The existing `cmd`/`internal` ownership remains. CI, tag-triggered draft-release
automation, contribution/security guides, review templates and operator Wiki
sources are implemented. Wiki sources are versioned under `docs/wiki` and published
separately. Source setup and the authorized bootstrap URL are documented.
The public repository is created, `origin` is configured, Wiki/private security
reporting are enabled, and the Wiki's seven pages plus sidebar are published at
`c44380910de1689d19559c54290012ab23846d07`. The browser verified its navigation.
`actionlint` 1.7.12 passed both workflows, the YAML hook passed, both versioned
macOS architectures built, and 24 Markdown files/local links plus 14 JSON
examples passed validation. The CI commit passed mandatory formatting, vet,
race and message hooks. A duplicate standalone quality run was interrupted so
the mandatory hook run could finish. No check was weakened or bypassed.
The normal pre-push hook passed all file checks and the Go quality gate, then
published the complete existing history and three conventional atomic publication
commits. GitHub's public `main` initially matched local
`237b8294d699117bac68a26c38dbe916ecfda09a`. GitHub recognizes the MIT license.
Publication edits leave production Go, dependencies and installer bytes/modes
unchanged. The existing exact 7,255/7,255 statement proof remains the source
baseline. Fresh remote checks run independently on both macOS architectures,
with exact coverage on Apple Silicon. Consult the
[per-commit CI results](https://github.com/zaRizk7/harness-ctl/actions/workflows/ci.yml)
for their status. No binary release version has been selected, and tag-triggered
draft creation has not been exercised. No live harness/account mutation is
authorized for development verification. (User, 2026; Local workspace, 2026; GitHub, 2026)

| Code | Publication proof | Result |
| --- | --- | --- |
| PV1 | `gh repo view`, commit API, license API | PUBLIC, default `main`, matching published source SHA and MIT recognition |
| PV2 | `git push -u origin main` | Normal fast-forward publication after all configured pre-push checks. No history rewrite |
| PV3 | Wiki push, remote SHA, browser and byte comparison | Seven pages plus sidebar at `c44380910de1689d19559c54290012ab23846d07`, matching all eight source files |
| PV4 | Workflow, document and binary validation | `actionlint` 1.7.12 and YAML hook passed. 24 Markdown files/local links and 14 JSON examples passed. Both versioned macOS architectures built |
| PV5 | Security-reporting API | Private vulnerability reporting enabled |
| PV6 | Fresh local `make check` after publication | Formatting, vet and complete race suite passed. Manager package: 413.092 seconds |
| PV7 | First source-push Apple Silicon CI | Completed successfully. Exact 7,255/7,255 production statements, versioned build and artifact upload. [Job evidence](https://github.com/zaRizk7/harness-ctl/actions/runs/37890195915/job/113689156688) |
| PV8 | First complete source-push CI | Both Apple Silicon and Intel quality/race checks, versioned binaries and artifact uploads passed. [Run evidence](https://github.com/zaRizk7/harness-ctl/actions/runs/37890195915). Later workflow/doc commits have their own per-commit results |

(Local workspace, 2026; GitHub, 2026)

The first run warned that the pinned artifact action targeted deprecated Node 20.
Its official v7.0.2 action contract uses Node 24 and retains the archive, name,
path and retention inputs needed here. CI now pins its verified commit
`cf430e030ddbb5b0abf93d22962f4752f3646cd9`. Subsequent run results are linked above.
GitHub also reports macOS ARM runner capacity delays, which do not change the
tests or release scope. (GitHub, 2026; Local workspace, 2026)

The publication series uses conventional atomic commits with empty bodies and
no co-author trailers. Every commit and source push passes the configured hooks.
The Wiki keeps its native separate Git history. (Local workspace, 2026)

## Current fix checkpoint, 2026-10-09

The request ledger and [remaining boundaries](limitations.md) are authoritative
over historical completion claims below.
Independent launch rules, captured Pi manifest compatibility, retained HOME skill
availability, approved shell PATH setup, self-removal owner/permanent controls,
read-only Google export costs and native subscription quota reporting are now
implemented. State codecs and local component planning have separate modules.
The non-race aggregate suite passed. Regression-first schema validation, unchanged
legacy policy normalization, native source validation and parked-only skills now
pass. Contract comments and operator guides describe current behavior. The full
race check passed. Both macOS architectures built, and the isolated PTY smoke
passed setup/PATH/navigation/self-removal. Strict coverage measured 7,254/7,255.
The invalid-export fixture was corrected to reach the missing validation path,
with its specific error asserted. The focused race test passed, and the unchanged
strict race-enabled gate passed at exactly 7,255/7,255 production statements.
Ten dependency-ordered conventional commits record the implementation and
documentation. Every commit passed its configured hooks, including formatting,
vet, race tests and message validation. The final all-files hook run passed.
All 197 versioned files matched the saved source/documentation snapshot before
this checkpoint update. Source bytes and modes remain unchanged.
The ledger still marks C3 and A1 partial and limits C4 to its verified native
adapter. Remaining project work and vendor constraints are separate entries in
the limitations guide. Live harness/account mutations remain excluded from
development verification. Repository/Wiki publication is separately authorized.
(User, 2026; Local workspace, 2026)

| Code | Current verification | Result |
| --- | --- | --- |
| V1 | `make check` | Formatting, vet and full race suite passed. Manager package: 416.692 seconds |
| V2 | `make coverage` | Unchanged strict race-enabled gate passed at exactly 7,255/7,255 production statements |
| V3 | Commit hooks and `pre-commit run --all-files` | All applicable checks passed, including conventional message format and empty bodies |
| V4 | `go build -trimpath` for `darwin/arm64` and `darwin/amd64` | Both passed |
| V5 | Isolated PTY smoke | Setup approval, binary/symlink, PATH preservation, user catalog, F2 monitoring, navigation and state-retaining self-uninstall passed |
| V6 | Snapshot comparison | All 197 versioned inputs matched before checkpoint finalization. Production source remains at the verified bytes and modes |

The gates ran sequentially with the user-installed Go toolchain,
`GOTOOLCHAIN=local` and workspace-local caches. The all-files hook took longer
than previous checks. Its active Go driver was inspected read-only, and the
command subsequently completed successfully without a source or check change.
No cause for that timing variation was established. This does not broaden the
synthetic verification into live vendor compatibility. (Local workspace, 2026)

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

The project adapts these instructions for its use case: repository skills live under
`.agents/skills/`, native role configurations under `.codex/agents/`, and `AGENTS.md`
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
cover verified Claude plugins, Gemini extensions and Pi npm packages. Local
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

## Historical expanded implementation checkpoint

This section records the expanded implementation before the current audit fixes
and commit series. Verification uses synthetic homes, fake native
runners, fixture HTTP clients and in-memory identities. No real harnesses,
provider accounts or services have been mutated. Publication is deferred until
the user supplies and authorizes the GitHub destination. (Local workspace, 2026)

| Code | Requirement | Implementation at that checkpoint |
| --- | --- | --- |
| A1 | Hide unavailable operations/state controls | Capability-based actions, current-resource preservation options, compatible library target selection |
| A2 | External providers without identity coupling | Configurable launch environment, report adapter and credential selection, shipped OpenRouter credit contract |
| A3 | Shared reusable umbrella | Encrypted library CRUD, explicit per-harness recipes and approved sequential application with recovery |
| A4 | Interactive/headless installation | HTTPS bootstrap with mandatory publisher checksum, setup TUI/CLI, private prefix, optional verified symlink |
| A5 | Installed catalog owned by user | Explicit non-overwriting setup, read-only startup from user catalog/configuration |
| A6 | Secure automatic launch behavior | Sandbox/reviewer defaults where verified, native policy plus labelled limitations otherwise |
| A7 | Track native additions | Native plugin ledgers, settings and asset manifests, including manually installed disabled plugins |
| A8 | Marketplace management | Supported native add, source edit, refresh/remove and local inventory, dependent plugin recovery scope |
| A9 | Monitoring throughout TUI | Compact reports by default, F2 complete details, one polling path defaulting to five seconds |
| A10 | Functionality-owned modules | Separate catalog, providers, storage, vault, component values, library and setup packages |
| A11 | Self-removal and batches | Independent state/harness choices, recorded launcher ownership and one approved sequential batch |
| A12 | Documentation and license | MIT license, operator guides in docs, standard Go comments and agnostic native role instructions |
| A13 | 100% test coverage | Passed the strict single-run race gate: 6,673 of 6,673 production statements covered |

Observed regression failures covered missing catalog startup/setup, protected
provider environment, negative key limits, absent native account links, manual
plugin ledgers, shared library application and recorded launcher removal. Focused
regressions and the normal full suite passed after implementation.
(Local workspace, 2026)

The final failure-contract work exposed a recovery defect: a failed replacement
followed by a failed rollback rename could delete the sole original payload.
`replaceTree` now retains that staging directory and reports its path. The
regression failed before the fix and passed afterward. Additional contracts
exercise cancellation of an already-exited process group, staging and service
failures, authenticated malformed archives, changed batches, component identity
and UI failure retention. Redundant checks after validated inventory and catalog
selection were removed while retaining trust-boundary validation.
(Local workspace, 2026)

## Historical verification before the audit, 2026-10-09

The commands use the user-installed Go toolchain with `GOTOOLCHAIN=local` and
workspace-local Go/pre-commit caches. Concurrent quality and coverage runs reached
Go's ten-minute whole-suite timeout during tests that had just started. The
sequential quality rerun passed without changing code, checks or timeout.
(Local workspace, 2026)

| Code | Check | Result |
| --- | --- | --- |
| V1 | `make check` | Passed formatting, vet and the complete race suite. Manager package: 409.480 seconds |
| V2 | `make coverage` | Passed one race-enabled aggregate profile: exactly 6,673 of 6,673 statements, 100%. Manager package: 409.040 seconds |
| V3 | Directive hooks over tracked and untracked existing files | All applicable hooks passed, including Go formatting, vet and race tests. Broken-symlink check had no applicable files |
| V4 | `go build -trimpath` for `darwin/arm64` and `darwin/amd64` | Both passed |
| V5 | Rebuilt arm64 binary in an isolated pseudo-terminal | Passed setup approval, verified binary/symlink, user catalog, F2 monitoring, library/accounts/batch/self navigation, clean exit and self-uninstall retaining state |
| V6 | Documentation-comment audit and `git diff --check` | Every production Go function has a comment. No whitespace errors |

The smoke test used a fresh synthetic home and performed no live harness,
provider, Keychain or service mutations. These results do not establish live
vendor compatibility. These were historical results before the audit and subsequent fixes. They do not
establish current completion. Consult the current checkpoint and request ledger.
Publication remains deferred.
(Local workspace, 2026)

## Accepted decisions

- D1: OpenAI, Anthropic and Google API/subscription information, with native links
  for plan changes. External providers use configurable contracts.
- D2: One batch approval, sequential safe execution under one mutation lock.
- D3: Prefer secure automatic approval. Use labelled native policy where a secure
  automatic contract is not verified. No unrestricted bypass flag by default.
- D4: The umbrella is a reusable library with approved application to selected
  harnesses. It does not silently propagate edits into applied copies.
- D5: Repository/Wiki publication is authorized at `zaRizk7/harness-ctl`. Binary
  assets await a selected release version. Installer URL/checksum remain explicit.

(User decisions, 2026)

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
- Provider API reports can lag. Optional OpenAI native quota reporting and Google
  export costs are implemented. Other consumer numeric adapters remain unverified
  project work. Invoices and paid plan changes use account pages. A five-second
  local refresh does not guarantee fresh upstream data. See `docs/accounts.md`.
- No binary release version has been selected or published. Source setup and
  the authorized bootstrap destination are documented. Setup requires an explicit
  HTTPS binary URL and trusted publisher checksum. See `docs/setup.md`.
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

- Local workspace (2026). [Component tests](../internal/manager/components_test.go),
  [TUI tests](../internal/manager/tui_test.go), [operator guide](components.md),
  [architecture](architecture.md), [agent instructions](../AGENTS.md),
  [recovery regression](../internal/manager/remaining_failure_contracts_test.go),
  [final boundary contracts](../internal/manager/last_boundary_contracts_test.go).
- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
