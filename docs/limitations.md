# Remaining boundaries

This table separates provider constraints from missing project adapters and
verification exclusions. It is part of the [request ledger](requests.md), not a
claim that unsupported features are impossible. (User, 2026)

| Code | Boundary | Classification | Current fallback or next requirement |
| --- | --- | --- | --- |
| R1 | Upstream report freshness and throttling | Provider constraint | Show fetched/cache/export times. Honor HTTP cooldowns. Anthropic recommends one-minute sustained polling and usually reports within five minutes. Google export initialization/backfill can take hours/days. A five-second UI cannot accelerate either source. |
| R2 | Authorized reporting access | Provider/account constraint | Supply eligible organization reporting keys or Cloud OAuth read access. Configure an existing export for Google costs and native subscription sign-in for app-server quotas. The manager does not grant privileges, refresh Cloud tokens or provision billing exports. |
| R3 | Anthropic/Google consumer subscription numeric reporting | Unimplemented/unverified project contract | Account records and native usage/billing links work. A documented or verified native machine-readable quota source is needed for an adapter. Lack of an adapter does not establish a vendor prohibition. OpenAI native quota reporting is implemented. |
| R4 | External plugin payload mutation and undisclosed component roots | Project scope and ownership boundary | Explicit `component_sources` provide read-only project/system inventory with provenance. Native ledgers include direct non-user additions, but an external payload is not claimed for deletion or rollback by its ledger alone. Import compatible copies through the library or use native scope controls. Unconfigured roots are not automatically scanned. |
| R5 | Other marketplaces and unknown plugin formats | Unverified project/vendor contracts | Claude and Codex adapters support native user-scope list/add/source-edit/refresh-or-upgrade/remove, with synthetic recovery proof. Other harnesses hide unavailable marketplace controls. Unknown manifests remain unverified. Add adapters only with verified loader/installer contracts. Compatibility is not license permission. |
| R6 | Automatic approval availability | Provider/native policy constraint | Use verified secure modes and labelled native policies. Claude auto availability depends on supported models, settings and server policy. Gemini auto-edit still prompts for other tools. Native sandbox/policy restrictions apply. Explicit native launch is available. No unrestricted bypass is a shipped fallback. |
| R7 | Native integrations and recovery scope | Verification and system boundary | Synthetic tests cover process, transport, key-store, service and installer contracts. Live vendor versions, Keychain prompts, account connections and service restarts remain untested. Snapshots restore captured local state, not remote changes, OS secrets or arbitrary installer side effects. |
| R8 | Binary signatures and platform installation proof | Unconfigured signing and verification boundary | v0.1.1 binaries and bootstrap are public. Both native macOS CI jobs passed. Actual public latest/pinned direct/symlink and interactive installation passed on Apple Silicon in disposable homes. Intel's public bootstrap was not locally exercised. Independent publisher signatures and Apple notarization remain unconfigured project work. |
| R9 | Native credential portability | Native storage and remote-session boundary | Opt-in encrypted profiles capture only declared native files. OS Keychain secrets remain native. Capture/restore does not perform OAuth, synchronize native refresh, or reverse remote revocation. Restored tokens may be expired or revoked. Authenticate natively when portability is unavailable. |

(Local workspace, 2026; Anthropic, 2026; Google, 2026; OpenAI, 2026)

The manager also blocks managed update/reinstall with owned LaunchAgents and
tracked Hermes update/reinstall until native rebinding/updater contracts are
verified. Remove the selected service registration through its native controls,
use the native updater, or migrate to a managed installation as the preview
permits. These are project adapter limits. (Local workspace, 2026)

## References

- User (2026). Accepted decisions in [requests](requests.md).
- Local workspace (2026). [Accounts](accounts.md), [components](components.md),
  [native authentication](authentication.md), [launch/setup controls](cli.md), [verification](testing.md),
  [checkpoint](implementation.md).
- Anthropic (2026). [Reporting freshness and polling](https://platform.claude.com/docs/en/manage-claude/usage-cost-api),
  [automatic approval availability](https://code.claude.com/docs/en/permission-modes).
- Google (2026). [Export setup and freshness](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-setup),
  [approval and sandbox configuration](https://geminicli.com/docs/reference/configuration/).
- OpenAI (2026). [Native account reporting](https://learn.chatgpt.com/docs/app-server),
  [sandbox/reviewer configuration](https://learn.chatgpt.com/docs/config-file/config-reference).
