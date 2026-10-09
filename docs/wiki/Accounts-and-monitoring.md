# Accounts and monitoring

Accounts belong to an independent encrypted vault. Removing a harness does not
remove them. Inference keys and reporting credentials have separate roles. Only
an explicitly selected inference key is injected into a harness launch. OpenAI,
Anthropic, Google and configurable external providers have distinct reporting
contracts. (Project, 2026)

## Add subscription metadata

Press `a` on the home screen to manage accounts. Add/edit uses a private JSON
editor. For CLI operation, save a local `subscription.json`:

```json
{
  "id": "personal",
  "provider": "openai",
  "kind": "subscription",
  "label": "Personal subscription",
  "enabled": true
}
```

```sh
harness-ctl accounts set subscription.json
harness-ctl accounts list
harness-ctl accounts open personal subscription
harness-ctl accounts monitor --watch personal
```

This record provides metadata and native account pages. For supported native
OpenAI quota windows, explicitly add `report_harness: "codex"` and optionally
`report_install_id` from `list`. The selected native source must already be
signed in to the intended subscription. Anthropic/Google consumer numeric
adapters remain unimplemented or unverified. Do not infer numeric limits from
plan labels. (Project, 2026)

## API keys, costs and billing

The [account schema](https://github.com/zaRizk7/harness-ctl/blob/main/docs/accounts.md)
shows API key/reporting fields and Google export configuration. Import secrets
from a private local file and remove that plaintext file afterward. Never submit
it in an issue or Wiki. Editor backups remain governed by your editor.
(Project, 2026)

```sh
harness-ctl accounts set account.json
harness-ctl accounts monitor --watch work-api
harness-ctl accounts open work-api usage
harness-ctl accounts open work-api billing
harness-ctl launch --account work-api codex
```

Paid plan changes and invoices open provider pages. The manager does not purchase
plans or revoke remote keys. Google costs need an existing readable billing
export and authorized reporting token. External providers configure identity,
key environment, HTTPS origins and reporting adapter separately. An OpenRouter
credit limit differs from rolling API cost and subscription quotas. (Project, 2026)

## TUI visibility and freshness

Compact monitoring appears throughout the TUI. Press **F2** for complete details.
The local refresh defaults to five seconds and `refresh_seconds` changes it.
CLI watch mode uses the same default and accepts `--refresh SECONDS` before the
account ID. Missing data stays unavailable, rather than appearing as zero.
(Project, 2026)

Five-second polling cannot make delayed provider reports real-time. Per-provider
fetch intervals, HTTP cooldowns and export freshness are shown separately. Check
the account guide's scope and freshness disclosures before interpreting figures
as session costs, remaining quota or an invoice. (Project, 2026)

## References

- Project (2026). [Accounts and reporting](https://github.com/zaRizk7/harness-ctl/blob/main/docs/accounts.md),
  [provider defaults](https://github.com/zaRizk7/harness-ctl/blob/main/internal/providers/providers.json),
  [limits](https://github.com/zaRizk7/harness-ctl/blob/main/docs/limitations.md).
