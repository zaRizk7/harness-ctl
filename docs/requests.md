# User request ledger

This is the durable acceptance record for the 23 requests beyond the original
plan. Preserve the codes across implementation, audits and compaction. Read this
file with [the checkpoint](implementation.md) before continuing work. The latest
user authorized documenting and resolving gaps after the read-only audit.
(User, 2026)

## Accepted decisions and original scope

- D1: macOS, Go and a TUI plus CLI. Go is installed by the user. Do not acquire a
  toolchain or mutate live harnesses/accounts during development verification.
- D2: Lifecycle includes install, uninstall, reinstall, factory reset and
  update/upgrade, with preservation/discard choices for local state categories.
- D3: Dedicated component screens support list, add/install, edit, enable,
  disable and remove for skills, MCP, plugins, connectors, proxies and hooks.
- D4: OpenAI, Anthropic and Google accounts. View usage, limits and billing. Paid
  plan changes use provider links. External provider contracts are configurable.
- D5: One batch approval, sequential safe execution. Completed items retain
  recovery when a later item fails.
- D6: Prefer secure automatic approval. Use native policy with a visible
  limitation where no verified secure automatic contract exists. Never introduce
  unrestricted bypass defaults as a substitute.
- D7: A reusable shared library applies compatible entries to selected harnesses
  after approval. Library edits do not silently change applied copies.
- D8: The release repository/URL will be supplied and publication authorized
  later. Do not publish, push or invent the release destination.
- D9: Follow the pinned coding directives, use native project role definitions,
  retain model/harness-agnostic implementation instructions, and omit commit
  co-author trailers. Delegation requires an explicit user request.

(User, 2026)

## Request checklist

Present means found in source. Historical means recorded verification, not a
current passing gate. Open means work owned by this project remains. External
means an upstream boundary has evidence and a documented fallback. Do not
relabel an unimplemented adapter as an unavoidable vendor limitation.
(Local workspace, 2026)

| Code | User requirement | Audit status and follow-up |
| --- | --- | --- |
| H1 | Uninstall harness-ctl with independent harness and manager-state choices | Implemented. CLI and TUI expose independent manager/harness state choices, shared-owner selection and permanent recovery erasure. Full race and strict aggregate coverage passed. |
| H2 | Batch harness operations, including install/uninstall of several harnesses | Present. Retain one approval and sequential locked execution. |
| H3 | Operate through a CLI without opening the TUI | Present. Keep CLI/TUI on the same approved transaction path. |
| H4 | Launch through harness-ctl with options and through native command names using PATH | Implemented. Shorthand, forwarding and shims share launch policy. Explicit setup --shell-file previews and approves a confined PATH append with stale checks and rollback. |
| H5 | Hide unavailable operations and state controls | Implemented, F4. Availability includes active and parked separate HOME skill roots. Parked-only regression passed independently. |
| H6 | Secure automatic launch defaults, with labelled native fallback | Implemented, F1. Independent catalog launch rules preserve security defaults for model-only overrides in direct launches and shims. Unchanged legacy policies normalize without rewriting user edits. |
| I1 | Remove the hard-coded Go harness catalog | Present. JSON metadata is configurable. Native format/install contracts remain adapter code. |
| I2 | Install catalog.json in the manager's user directory | Present. Non-overwriting setup and startup loading. |
| I3 | Curl bootstrap, setup TUI/headless, private prefix, symlink/direct install, secure flexible defaults | Present locally. Publication destination is deferred under D8. |
| C1 | Recognize harness-specific plugins/extensions and manage their native contracts | Implemented, F3. Imports and captured library files share manifest detection, including Pi. Unknown formats make no compatibility claim. |
| C2 | One umbrella for shareable components | Present. Encrypted reusable library with approved selected-harness application. Retain compatibility and recovery guards. |
| C3 | Track components installed outside harness-ctl | Partial. User-scope ledgers/assets are rescanned. Project/system inventory and external payload ownership are separate implementation/safety boundaries. Do not promise universal tracking. |
| C4 | Manage marketplaces where harnesses support them, including non-TUI operation | Present for the verified Claude adapter. List/add/source-edit/refresh/remove. Other adapters are unverified, not proven impossible. |
| A1 | Independent API keys/subscriptions, usage/limits/billing, real-time monitoring defaulting to 5s | Implemented in part, F2. Optional documented app-server subscription quota reporting and read-only Google billing export costs now exist. Required export schema and reporting-source identity now have passing regressions. Other consumer adapters remain unverified. Upstream freshness remains a provider constraint. |
| A2 | Configurable external providers such as OpenRouter without tight identity coupling | Present. Identity, reporting adapter, environment and credentials are configured separately. |
| A3 | Compact monitoring throughout the TUI with a key for complete information | Present UI. F2 details and global polling. Actual available data remains subject to A1. |
| D1 | Standard Go package/function documentation describing arguments and returns | Implemented, F6. Production functions have comments. Exported planning, codec, process, setup and key-store contracts now describe arguments/results and mutation boundaries. Guides describe current reporting/PATH behavior. |
| D2 | Documents under docs except summaries, licenses and primary directives | Present for operator/developer guides. Discovery-required skills/role configurations remain at native directive paths. |
| D3 | Convert subagent roles to native project configurations | Five native definitions parsed and validated with inherited models. Runtime delegation was not exercised because it was not authorized. |
| D4 | Model/harness-agnostic implementation-agent documentation | Present in instruction prose. Native paths and upstream provenance URLs retain their names. |
| D5 | MIT license | Present. |
| E1 | 100% production coverage and unit/integration testability | Passed the unchanged strict race-enabled gate at exactly 7,255/7,255 production Go statements. Unit, integration and synthetic PTY evidence is recorded below. Coverage does not prove every acceptance condition is correct or cover shell execution. |
| E2 | Self-contained functional modules and smaller manager footprint | Implemented, F5. Native codecs/classification/field traversal moved to stateconfig. Local component planning and parked-array contracts moved to component. Launch policy and app-server protocol have independent ownership. Manager retains transaction/owner validation. Full race and coverage gates passed. |

(User, 2026; Local workspace, 2026)

## Original audit findings to preserve

- F1: `defaultLaunchArgs` and generated shims suppress every default when any
  configured approval flag appears. A model-only `-c` must retain security policy.
- F2: Subscription reports currently supply metadata/page links. Google reports
  requests/quotas/billing linkage, without amounts. Separate missing adapters
  from upstream access requirements and freshness limits.
- F3: Local source imports detect Pi's `package.json` declaration. Captured file
  compatibility checks recognize only the other two supported manifest shapes.
- F4: `hasLocalState` omits the separate HOME skill root, hiding management for a
  harness whose only retained state is user skills.
- F5: Manager owns feature-specific inventory, planning and execution despite
  small support packages. Refactor one ownership boundary at a time with proof.
- F6: Comment presence does not establish complete Go contracts. The checkpoint
  and component capability table also have stale Pi support descriptions.

(Local workspace, 2026)

## Resumption checkpoint, 2026-10-09

The non-race aggregate audit passed all package tests at 7,178/7,208 statements
before final fixes. Regression-first fixes now cover required billing schema,
unchanged legacy policy normalization, native reporting-source validation,
parked-only skills and distinct self-owner navigation. Provider/app-server/setup
and stateconfig standalone suites have 100% statement coverage. The full race gate
passed, with manager tests at 416.692 seconds. Both macOS architectures built. The
synthetic PTY passed setup/PATH/navigation/self-removal. Strict aggregate coverage
found one missed statement at 7,254/7,255 despite rounded 100.0%. The invalid-export
fixture now clears its orphan report source and asserts the specific export error.
The focused race rerun passed. The unchanged strict gate then passed at exactly
7,255/7,255 production statements. Ten dependency-ordered conventional commits
record the implementation and documentation. Each commit passed its configured
formatting, vet, race and message hooks. The final all-files hook run also passed.
All 197 versioned files matched the saved source/documentation snapshot before
this checkpoint update. Source bytes and modes remain unchanged.
Publication remains deferred. C3 and A1 are partial, and C4 is verified for only
one native adapter. See [remaining boundaries](limitations.md) for project work,
provider constraints, unverified contracts and their fallbacks. These items must
not be treated as completed merely because the source coverage gate passed.
(User, 2026; Local workspace, 2026)

## Boundaries and evidence

Record each unresolved item as one of: project implementation work, unverified
vendor contract, vendor/provider constraint with an official source, deferred
user decision, or verification not performed. Include its fallback and what
would be needed to resolve it. Do not describe lack of implementation as an
upstream impossibility. Do not fabricate financial figures, scrape private
consumer endpoints, or equate polling frequency with upstream data freshness.
(User, 2026; Local workspace, 2026)

## References

- User (2026). Requests and accepted decisions in the implementation conversation.
- Local workspace (2026). [Checkpoint](implementation.md),
  [architecture](architecture.md), [accounts](accounts.md),
  [components](components.md), [CLI](cli.md), [setup](setup.md),
  [verification](testing.md), [remaining boundaries](limitations.md).
