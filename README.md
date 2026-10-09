# harness-ctl

A macOS TUI and CLI for managing coding harnesses, their local components,
launch profiles and provider accounts. Go is user-managed. Licensed under MIT.

Build with your installed Go toolchain:

```sh
GOTOOLCHAIN=local make build
./bin/harness-ctl
./bin/harness-ctl --help
```

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

## References

- Local workspace (2026). [Implementation checkpoint](docs/implementation.md).
- zaRizk7 (2026). [Coding directives](https://github.com/zaRizk7/claude-coding-directives),
  pinned at `dff6aa85dc2644bba4ea71b6c6d421bc7a260f74`.
