# Components and library

Select a harness and installation, then **Manage components**. Categories include
skills, MCP, plugins, connectors, proxies and hooks. `a` adds/installs, `e` edits,
`d` disables, `u` enables and `x` removes. Every component mutation uses an approved
preview. Press `p` to switch base/profile and `o` to select shared owners.
(Project, 2026)

## Register an MCP server

First list the selected state. Replace the path and server command below with
the concrete destination and installed executable for your installation. This
registers a server. It does not install or authorize that server. (Project, 2026)

```sh
harness-ctl components list codex mcp
```

Save `request.json`:

```json
{
  "operation": "add",
  "category": "mcp",
  "path": "/absolute/selected-state/config.toml",
  "field": "/mcp_servers/demo",
  "value": {"command": "/absolute/path/to/server", "args": [], "enabled": true}
}
```

```sh
harness-ctl components apply --preview codex request.json
harness-ctl components apply codex request.json
```

## Reuse an approved library recipe

Press `l` on the home screen for the encrypted shared library. A record declares
its category and compatible target recipes. Save this template as `entry.json`
after replacing the command with your own installed server. (Project, 2026)

```json
{
  "id": "demo-server",
  "category": "mcp",
  "enabled": true,
  "targets": {
    "codex": {"path": "config.toml", "field": "/mcp_servers/demo-server", "value": {"command": "my-server"}},
    "claude": {"path": "settings.json", "field": "/mcpServers/demo-server", "value": {"command": "my-server"}}
  }
}
```

```sh
harness-ctl library set --preview entry.json
harness-ctl library set entry.json
harness-ctl library apply --preview demo-server codex claude
harness-ctl library apply demo-server codex claude
```

Application refuses existing names and requires any affected owners. Each target
copy remains independent. Editing, disabling or removing the library record
does not silently change applied copies. Skill assets can be captured from a
local source using the
[asset recipe](https://github.com/zaRizk7/harness-ctl/blob/main/docs/library.md).
(Project, 2026)

## Native extensions and marketplaces

The manager rescans user-scope native ledgers and assets, including entries added
outside it. Native plugins/extensions use harness-specific contracts and detected
manifests. Unknown formats remain unverified. Loader compatibility does not grant
a proprietary license. Project/system sources and externally owned payloads do
not have universal management support. (Project, 2026)

The verified marketplace adapter provides list, add, source edit, refresh and
remove without opening the native harness TUI. Save a request with
`category: "marketplaces"`, `native: true`, `operation: "add"`, an expected
`name` and a reviewed `source`. Other marketplace adapters remain unverified.
(Project, 2026)

```sh
harness-ctl components list claude marketplaces
harness-ctl components apply --preview claude marketplace.json
harness-ctl components apply claude marketplace.json
```

Disabled entries may be parked as plaintext local state in private directories.
Encrypted recovery covers captured local files, not remote OAuth, OS-held secrets
or arbitrary installer side effects. Read the
[capability table and request schema](https://github.com/zaRizk7/harness-ctl/blob/main/docs/components.md)
before importing native code. (Project, 2026)

## References

- Project (2026). [Components](https://github.com/zaRizk7/harness-ctl/blob/main/docs/components.md),
  [library](https://github.com/zaRizk7/harness-ctl/blob/main/docs/library.md),
  [limitations](https://github.com/zaRizk7/harness-ctl/blob/main/docs/limitations.md).
