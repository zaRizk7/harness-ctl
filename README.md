# harness-ctl

[![CI](https://github.com/zaRizk7/harness-ctl/actions/workflows/ci.yml/badge.svg)](https://github.com/zaRizk7/harness-ctl/actions/workflows/ci.yml)

A macOS TUI and CLI for managing coding harnesses, their local components,
launch profiles and provider accounts. Go is user-managed. Licensed under MIT.

Install, uninstall, reinstall, reset and update selected harnesses through one
approved sequential batch. Manage skills, MCP, native plugins, connectors,
proxies and hooks with local recovery, or reuse compatible recipes through the
shared library. Independent account monitoring stays visible across the TUI.
(Project, 2026)

Build from source with the Go version in `go.mod`. A binary release has not yet
been published. Start with the [Wiki installation tutorial](https://github.com/zaRizk7/harness-ctl/wiki/Installation):

```sh
git clone https://github.com/zaRizk7/harness-ctl.git
cd harness-ctl
GOTOOLCHAIN=local make build
./bin/harness-ctl
./bin/harness-ctl --help
```

The [Wiki](https://github.com/zaRizk7/harness-ctl/wiki) walks through setup,
batches, components, accounts and recovery. Full contracts and limitations remain
versioned with the code:

1. [Installation](docs/setup.md): verified release setup and user-owned catalog.
2. [Shared library](docs/library.md): reusable components and approved application.
3. [Operator guide](docs/usage.md): lifecycle, preservation, recovery and keyboard controls.
4. [CLI and batch guide](docs/cli.md): non-TUI operations, launching, PATH and self-uninstall.
5. [Components](docs/components.md): skills, MCP, native extensions, connectors, hooks and proxies.
6. [Accounts](docs/accounts.md): keys, subscriptions, billing and five-second monitoring.
7. [Architecture](docs/architecture.md): code ownership, transactions and Go API contracts.
8. [Acceptance checkpoint](docs/implementation.md): verification evidence and material limits.
9. [Verification](docs/testing.md): unit/integration boundaries and strict coverage target.
10. [User request ledger](docs/requests.md): all post-plan requests, decisions and open gaps.
11. [Remaining boundaries](docs/limitations.md): provider constraints, project gaps and untested live integrations.

Implementation agents follow [AGENTS.md](AGENTS.md). Native role configurations
inherit the active model. Delegation requires an explicit request. Development
checks use synthetic state and never modify live harness installations.

```sh
make hooks
make check
make coverage
```

See [contribution guidance](docs/CONTRIBUTING.md), [security reporting](docs/SECURITY.md)
and [maintainer publishing](docs/releasing.md). The layout uses `cmd/harness-ctl`
for the executable, functionality-owned `internal` packages, `scripts` for
automation and `docs` for guides and Wiki sources. CI verifies both macOS
architectures and enforces exact production statement coverage. (Project, 2026)

Consumer subscription reporting, project/system component inventory and other
marketplace adapters still have explicit gaps. Source tests use synthetic
boundaries, so live vendor compatibility remains unverified. Review
[remaining boundaries](docs/limitations.md) before relying on those integrations.
(Project, 2026)

## References

- Project (2026). [Architecture](docs/architecture.md), [CI](.github/workflows/ci.yml),
  [limitations](docs/limitations.md).
- Local workspace (2026). [Implementation checkpoint](docs/implementation.md).
- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
