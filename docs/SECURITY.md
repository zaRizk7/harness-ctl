# Security reporting

Report suspected vulnerabilities through the repository's **Security → Report a
vulnerability** form when private reporting is available. If it is unavailable,
open an issue requesting a private contact without disclosing exploit details,
credentials or user state. Do not upload account vaults, recovery archives,
authentication files or component configuration. (Project policy, 2026)

Include the affected commit/version, the violated boundary, a minimal synthetic
reproduction and the expected result. Redact paths and account identifiers.
No released version has a maintenance commitment yet. Fixes are reviewed on
`main` until a release policy is adopted. (Project policy, 2026)

The implementation requires concrete approval for mutations, revalidates path
ownership and fingerprints under a lock, and uses encrypted authenticated
recovery. Native installers, OS-held secrets and provider-side effects are
outside complete snapshot recovery. Review [limitations](limitations.md) and
[recovery](usage.md#recovery) when assessing a report. Statement coverage does
not prove that every security condition or live vendor version is correct.
(Local workspace, 2026)

## References

- Project policy (2026). This repository's reporting and maintenance policy.
- Local workspace (2026). [Transaction architecture](architecture.md),
  [limitations](limitations.md), [verification](testing.md).
