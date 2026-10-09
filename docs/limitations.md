# Remaining boundaries

This table separates provider constraints from missing project adapters and
verification exclusions. It is part of the [request ledger](requests.md), not a
claim that unsupported features are impossible. (User, 2026)

| Code | Boundary | Classification | Current fallback or next requirement |
| --- | --- | --- | --- |
| R1 | Upstream report freshness and throttling | Provider constraint | Show fetched/cache/export times. Honor HTTP cooldowns. Anthropic recommends one-minute sustained polling and usually reports within five minutes. Google export initialization/backfill can take hours/days. A five-second UI cannot accelerate either source. |
| R2 | Authorized reporting access | Provider/account constraint | Supply eligible organization reporting keys or Cloud OAuth read access. Configure an existing export for Google costs and native subscription sign-in for app-server quotas. The manager does not grant privileges, refresh Cloud tokens or provision billing exports. |
| R3 | Anthropic/Google consumer subscription numeric reporting | Unimplemented/unverified project contract | Account records and native usage/billing links work. A documented or verified native machine-readable quota source is needed for an adapter. Lack of an adapter does not establish a vendor prohibition. OpenAI native quota reporting is implemented. |
| R4 | Project/system component inventory and external plugin payloads | Project scope and ownership boundary | User/base/profile resources and native ledgers are rescanned, including direct vendor additions. Project/system screens require explicit source roots and reviewed ownership. External payloads are not claimed for deletion or rollback by their ledger alone. Universal tracking is not established. |
| R5 | Other marketplaces and unknown plugin formats | Unverified project/vendor contracts | The verified marketplace adapter supports native user-scope list/add/source-edit/refresh/remove. Other harnesses hide unavailable marketplace controls. Unknown manifests remain unverified. Add adapters only with verified loader/installer contracts. Compatibility is not license permission. |
| R6 | Automatic approval availability | Provider/native policy constraint | Use verified secure modes and labelled native policies. Claude auto availability depends on supported models, settings and server policy. Gemini auto-edit still prompts for other tools. Native sandbox/policy restrictions apply. Explicit native launch is available. No unrestricted bypass is a shipped fallback. |
| R7 | Native integrations and recovery scope | Verification and system boundary | Synthetic tests cover process, transport, key-store, service and installer contracts. Live vendor versions, Keychain prompts, account connections and service restarts remain untested. Snapshots restore captured local state, not remote changes, OS secrets or arbitrary installer side effects. |
| R8 | Binary release version and distribution | Deferred release decision and verification not performed | Public source and Wiki are published at zaRizk7/harness-ctl. Source builds and the HTTPS bootstrap are documented. No binary release version has been chosen or published. Tag automation prepares a draft for review but has not been exercised with a release tag. Signing/notarization remain unconfigured. |

(Local workspace, 2026; Anthropic, 2026; Google, 2026; OpenAI, 2026)

The manager also blocks managed update/reinstall with owned LaunchAgents and
tracked Hermes update/reinstall until native rebinding/updater contracts are
verified. Remove the selected service registration through its native controls,
use the native updater, or migrate to a managed installation as the preview
permits. These are project adapter limits. (Local workspace, 2026)

## References

- User (2026). Accepted decisions in [requests](requests.md).
- Local workspace (2026). [Accounts](accounts.md), [components](components.md),
  [launch/setup controls](cli.md), [verification](testing.md),
  [checkpoint](implementation.md).
- Anthropic (2026). [Reporting freshness and polling](https://platform.claude.com/docs/en/manage-claude/usage-cost-api),
  [automatic approval availability](https://code.claude.com/docs/en/permission-modes).
- Google (2026). [Export setup and freshness](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-setup),
  [approval and sandbox configuration](https://geminicli.com/docs/reference/configuration/).
- OpenAI (2026). [Native account reporting](https://learn.chatgpt.com/docs/app-server),
  [sandbox/reviewer configuration](https://learn.chatgpt.com/docs/config-file/config-reference).
