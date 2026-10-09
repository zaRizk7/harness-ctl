# Provider accounts and monitoring

Accounts are independent of harness installations. Add inference keys,
organization reporting credentials or subscription metadata for OpenAI,
Anthropic, Google and configurable external providers, including OpenRouter. The vault encrypts keys before atomic publication. Its
identity lives in the native credential store. Listing, previews and monitoring
output omit credential values. (Local workspace, 2026)

The TUI home screen's `a` opens accounts. Add/edit uses a private JSON editor
request, with existing keys omitted. Blank keys preserve existing credentials.
Enable/disable controls monitoring and launch-key selection. Remove deletes only
local account records. Provider key revocation and paid plan changes use native
account pages. Account previews check vault freshness under the mutation lock.
Changing providers requires replacing each previously stored inference/reporting
credential, preventing reuse against a different provider. (Local workspace, 2026)

```json
{
  "id": "work-api",
  "provider": "openai",
  "kind": "api",
  "label": "Work API",
  "credential": "REPLACE_WITH_INFERENCE_KEY",
  "monitor_credential": "REPLACE_WITH_REPORTING_CREDENTIAL",
  "organization": "org-example",
  "project": "proj-example",
  "enabled": true
}
```

Save a private file, import it, then remove that plaintext file yourself. Editor
requests are removed by the manager's normal callback. Editor backups and swap
files remain governed by your editor. Subscriptions use `kind: "subscription"`
and optional `plan`/`notes` metadata. No credential is needed to open their pages.
(Local workspace, 2026)

```sh
harness-ctl accounts set account.json
harness-ctl accounts list
harness-ctl accounts disable work-api
harness-ctl accounts enable work-api
harness-ctl accounts monitor --watch work-api
harness-ctl accounts monitor --refresh 10 work-api
harness-ctl accounts open work-api usage
harness-ctl accounts open work-api billing
harness-ctl accounts open work-api keys
harness-ctl accounts open personal subscription
harness-ctl accounts remove work-api
harness-ctl launch --account work-api codex
```

Reporting/admin credentials never become harness launch credentials. Explicit
account selection supplies only its inference key through the provider's native
environment name. Removing a harness does not remove provider accounts.
`self-uninstall --state` permanently discards the manager vault.
(Local workspace, 2026)

| Provider/account | Programmatic data | Credential/access | Native pages |
| --- | --- | --- | --- |
| OpenAI API | Organization completions usage, organization costs, project rate limits when a project is selected | Organization admin/reporting credential | Keys, usage/limits, billing and invoices |
| Anthropic API | Organization message token usage, costs and organization rate limits | Organization Admin API credential with reporting access | Keys, limits, billing and invoices |
| Google API | Request counts, quota limits, billing linkage and optional net project costs from an existing billing export | Google Cloud OAuth reporting token, project ID and export read access | Keys, invoices and plan changes |
| OpenAI subscription | Optional native quota windows, percentage used and reset time | Explicit native report source with an existing subscription sign-in | Billing, invoices and plan changes |
| Other consumer subscriptions | Local label/plan metadata and provider links | Native sign-in | Usage, plan limits, invoices and plan changes. Numeric adapters remain unverified project work |

The default refresh is **5 seconds**. `refresh_seconds` configures TUI and CLI
polling. A compact report is visible throughout the TUI. F2 toggles complete details. A single poller runs across screens, independently of account-preview freshness. `monitor --watch` refreshes
until interrupted. `monitor_window_days` defaults to 30 and `monitor_max_pages`
to 100. Paginated reports are aggregated, with incomplete reports labelled as
errors. Unavailable data is not reported as zero usage. (Local workspace, 2026)

HTTP 429 pauses reporting requests for that account across both interfaces. The
manager honors `Retry-After` seconds or dates and uses the configured refresh
interval when the header is absent or invalid. Reports show the remaining pause.
Watch mode rereads account changes on every cycle. (Local workspace, 2026)

Provider reports can lag. Anthropic recommends sustained polling once a minute,
with reports typically arriving within five minutes. Its shipped
`min_report_seconds` is 60. Google also uses a configurable 60-second fetch
interval to bound repeated export scans. Five-second screen refreshes show cached
fetch times and the next provider fetch. HTTP throttling remains independent.
(Anthropic, 2026; Local workspace, 2026)

Google export initialization may take hours and retroactive data may take five
days to catch up. The latest matching export timestamp indicates an observation,
not a guarantee of complete upstream data. Organization/API totals, consumer
quota percentages and harness-session counters describe different scopes.
(Google, 2026; Local workspace, 2026)

## Native subscription quota reporting

To read OpenAI subscription limits, select an installed Codex source explicitly.
The report uses its existing native sign-in and selected profile/state root.
It does not copy credentials, start an inference turn, consume a reset credit or
change the subscription. The provider contract must select
`subscription_adapter: "app_server"`, as the shipped OpenAI default does.
(Local workspace, 2026)

```json
{
  "id": "personal",
  "provider": "openai",
  "kind": "subscription",
  "report_harness": "codex",
  "report_install_id": "OPTIONAL_INSTALL_ID_FROM_LIST",
  "enabled": true
}
```

Omit `report_install_id` to select the active matching installation. A bounded
local JSONL process initializes, then reads `account/rateLimits/read`. Reports
prefer named buckets and show available usage percentages, window durations and
reset timestamps. Missing fields remain unavailable. Source labels are local
metadata, so select the sign-in/profile for the intended account. Invoices and
plan changes use account links. Other consumer subscription adapters are not
implemented or verified. This is a project gap, not proof of a vendor prohibition.
(OpenAI, 2026; Local workspace, 2026)

## Google export costs

Add `billing_export` to a Google API record whose `project` identifies the
project being monitored. The export project can differ from that project.
`service_ids` optionally limits costs to selected billing service IDs.
(Local workspace, 2026)

```json
{
  "id": "cloud",
  "provider": "google",
  "kind": "api",
  "project": "monitored-project",
  "monitor_credential": "READ_ONLY_CLOUD_OAUTH_TOKEN",
  "billing_export": {
    "project": "billing-export-project",
    "dataset": "billing",
    "table": "gcp_billing_export_v1_ACCOUNT",
    "service_ids": []
  },
  "enabled": true
}
```

The manager reads the existing standard/detailed export schema and table pages
using GET. It never creates query jobs or enables billing exports. Net USD is
row cost plus credits, converted with that row's exchange rate, filtered by
project, optional services and usage start time in the configured window.
Without service filtering the figure includes every exported service in that
project, so it is labelled project cost rather than inferred model cost.
(Google, 2026; Local workspace, 2026)

Required fields/types are checked before scanning. Changing page totals, invalid
values, repeated cursors or configured byte/page limits leave cost unavailable.
A completed scan totals the exported rows only. Concurrent export updates and
upstream delays prevent treating it as a live invoice. Supply an authorized
OAuth token and an existing readable export. Token acquisition/refresh and export
provisioning remain native. (Local workspace, 2026)

Provider endpoint defaults are [data](../internal/providers/providers.json) and
can be configured through `providers`. Changing API origins explicitly changes
where reporting credentials are sent. Endpoints require HTTPS and account
requests refuse redirects. Errors withhold provider response bodies.
(Local workspace, 2026)

## External providers

Provider identity, inference environment and reporting adapter are separate
configuration fields. Custom providers may omit reporting or account pages. A
missing reporting contract never sends a key. Configure `launch_env` for the
selected harness's documented endpoint names, and `key_env` for its inference
key. This does not translate a harness's model-selection syntax. Process/state
environment overrides are rejected. (Local workspace, 2026)

OpenRouter ships with key credit reporting through `GET /api/v1/key`. Its
all-time credits, credit limit and remaining credits are separate from token
usage, rolling API dollar costs and consumer subscription limits. A null limit
is unlimited. It explicitly uses its inference key for this endpoint. Other
reporting adapters retain separate admin/reporting credentials. (OpenRouter, 2026)

The reusable `key_usage` adapter maps an `object_pointer`, `cost_field`,
`limit_field`, `remaining_field` and `unit` from a provider's HTTPS JSON response.
Configure it only against the provider's verified contract. Provider URLs and
key-introspection field names live in user configuration. (Local workspace, 2026)

## References

- Local workspace (2026). [Vault](../internal/manager/accounts.go),
  [monitoring](../internal/providers/monitor.go), [CLI](../internal/manager/accounts_cli.go),
  [tests](../internal/manager/accounts_test.go).
- OpenAI (2026). [App-server account reports](https://learn.chatgpt.com/docs/app-server),
  [Organization usage](https://developers.openai.com/api/reference/resources/organization/subresources/usage/).
- Anthropic (2026). [Usage and Cost API](https://platform.claude.com/docs/en/manage-claude/usage-cost-api).
  [Rate limits API](https://platform.claude.com/docs/en/manage-claude/rate-limits-api).
- Google (2026). [Cloud Monitoring](https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.timeSeries/list),
  [consumer quotas](https://cloud.google.com/service-usage/docs/reference/rest/v1beta1/services.consumerQuotaMetrics/list),
  [project billing](https://cloud.google.com/billing/docs/reference/rest/v1/projects/getBillingInfo),
  [Gemini rate limits](https://ai.google.dev/gemini-api/docs/rate-limits),
  [export setup and freshness](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-setup),
  [standard export schema](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-tables/standard-usage),
  [tabledata.list](https://docs.cloud.google.com/bigquery/docs/reference/rest/v2/tabledata/list).
- OpenRouter (2026). [Credits and rate limits](https://openrouter.ai/docs/api/reference/limits).
