# Remaining boundaries

This table separates provider constraints from missing project adapters and
verification exclusions. It is part of the [request ledger](requests.md), not a
claim that unsupported features are impossible. (User, 2026)

| Code | Boundary | Classification | Current fallback or next requirement |
| --- | --- | --- | --- |
| R1 | Upstream report freshness and throttling | Provider constraint | Show fetched/cache/export times. Honor HTTP cooldowns. Anthropic recommends one-minute sustained polling and usually reports within five minutes. Google export initialization/backfill can take hours/days. A five-second UI cannot accelerate either source. |
| R2 | Authorized reporting access | Provider/account constraint | Supply eligible organization reporting keys or Cloud OAuth read access. Configure an existing export for Google costs and native subscription sign-in for app-server quotas. The manager does not grant privileges, refresh Cloud tokens or provision billing exports. |
| R3 | Consumer subscription report availability | Native access/version constraint and remaining project contracts | OpenAI supports native quotas and optional account token activity. Anthropic supports the experimental SDK control contract when initialization confirms a runtime version at or above the configured minimum (shipped: 2.1.296). It requires eligible native sign-in and omits local transcript scans and session cost estimates. User-authorized live native subscription reporting passed with existing sign-ins after the default namespace fix. Organization policy and other account/version combinations remain untested. Google consumer reporting remains unimplemented/unverified project work. Use native usage and billing pages when data is unavailable. |
| R4 | External plugin payload mutation and undisclosed component roots | Project scope and ownership boundary | Explicit `component_sources` provide read-only project/system inventory with provenance. Native ledgers include direct non-user additions, but an external payload is not claimed for deletion or rollback by its ledger alone. Import compatible copies through the library or use native scope controls. Unconfigured roots are not automatically scanned. |
| R5 | Other marketplaces and unknown plugin formats | Unverified project/vendor contracts | Claude and Codex adapters support native user-scope list/add/source-edit/refresh-or-upgrade/remove, with synthetic recovery proof. Other harnesses hide unavailable marketplace controls. Unknown manifests remain unverified. Add adapters only with verified loader/installer contracts. Compatibility is not license permission. |
| R6 | Automatic approval availability | Provider/native policy constraint | Use verified secure modes and labelled native policies. Claude auto availability depends on supported models, settings and server policy. Gemini auto-edit still prompts for other tools. Native sandbox/policy restrictions apply. Explicit native launch is available. No unrestricted bypass is a shipped fallback. |
| R7 | Native integrations and recovery scope | Verification and system boundary | Synthetic tests cover process, transport, key-store, service and installer contracts. Existing native subscription auth and an existing OpenRouter key passed user-authorized live reporting checks. Other vendor versions, Keychain prompts, lifecycle changes and service restarts remain untested. Snapshots restore captured local state, not remote changes, OS secrets or arbitrary installer side effects. |
| R8 | Binary signatures and platform installation proof | Unconfigured signing and verification boundary | v0.2.1 binaries and bootstrap are public. Both native macOS CI jobs passed. Actual public latest/pinned direct/symlink and interactive installation passed on Apple Silicon in disposable homes. Intel's public bootstrap was not locally exercised. Independent publisher signatures and Apple notarization remain unconfigured project work. |
| R9 | Native credential portability | Native storage and remote-session boundary | Opt-in encrypted profiles capture only declared native files. OS Keychain secrets remain native. Capture/restore does not perform OAuth, synchronize native refresh, or reverse remote revocation. Restored tokens may be expired or revoked. Authenticate natively when portability is unavailable. |
| R10 | SDK report stability and accounting scope | Experimental native contract and provider accounting boundary | Anthropic SDK control data can change or be absent. OpenAI token activity is separate from API billing. OpenRouter key totals, period spend and effective budgets use provider accounting scopes. Missing fields remain unavailable. The manager does not create inference turns or infer billed charges from session estimates. |

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
  [automatic approval availability](https://code.claude.com/docs/en/permission-modes),
  [SDK report types](https://code.claude.com/docs/en/agent-sdk/typescript),
  [SDK releases](https://github.com/anthropics/claude-agent-sdk-typescript/releases).
- Google (2026). [Export setup and freshness](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-setup),
  [approval and sandbox configuration](https://geminicli.com/docs/reference/configuration/).
- OpenAI (2026). [Native account reporting](https://learn.chatgpt.com/docs/app-server),
  [sandbox/reviewer configuration](https://learn.chatgpt.com/docs/config-file/config-reference).
- OpenRouter (2026). [Key usage and budgets](https://openrouter.ai/docs/api/api-reference/api-keys/get-current-api-key).
