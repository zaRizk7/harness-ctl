# Shared component library

Manage reusable skills, MCP registrations, plugins, connectors, proxies, hooks
and other supported local components from one encrypted library. Each entry
explicitly declares its compatible harness recipes. Application requires a
separate preview and approval for selected harnesses. Target copies stay
independent, so editing or disabling a library entry does not silently change
existing harness state. (Local workspace, 2026)

Home-screen `l` opens the library. Use `a` to add, `e` to edit, `d` to disable,
`u` to enable and `x` to remove. Enter chooses compatible installed harnesses or
harnesses with retained local state. Space selects targets, `o` selects affected
owners, and Enter previews the ordered batch. Type `apply` to approve.
(Local workspace, 2026)

```sh
harness-ctl library list
harness-ctl library set entry.json
harness-ctl library disable my-server
harness-ctl library enable my-server
harness-ctl library apply --preview my-server codex claude
harness-ctl library apply --owners 'EXACT OWNER FROM PREVIEW' my-server codex
harness-ctl library remove my-server
```

Registration example:

```json
{
  "id": "my-server",
  "category": "mcp",
  "enabled": true,
  "targets": {
    "codex": {
      "path": "config.toml",
      "field": "/mcp_servers/my-server",
      "value": {"command": "my-server"}
    },
    "claude": {
      "path": "settings.json",
      "field": "/mcpServers/my-server",
      "value": {"command": "my-server"}
    }
  }
}
```

Asset example:

```json
{
  "id": "my-skill",
  "category": "skills",
  "enabled": true,
  "source": "/absolute/local/skill-directory",
  "targets": {
    "codex": {"path": "skills/my-skill", "home_skills": true},
    "claude": {"path": "skills/my-skill"}
  }
}
```

`source` is captured on import. The vault keeps independent file bytes, and the
source path is removed from the saved record. Symlinks and nonregular assets are
rejected. `files` represents captured bytes through JSON base64 values and is
primarily useful for editing an existing record. Application creates owned local
copies and refuses existing component names. Use component editing for an
existing target rather than silently replacing it. (Local workspace, 2026)

Native recipes use `native`, `name` and `source` with the harness-specific plugin
contracts. A manifest for a proprietary plugin limits compatible destinations.
Native dependency scripts, remote authorization and OS-held secrets keep their
native recovery limits. The source format is validated, but a compatible recipe
is not a guarantee that upstream code is safe. (Local workspace, 2026)

A batch holds one mutation lock, rechecks the library vault and every target,
applies entries sequentially and records encrypted recovery per harness. Failure
rolls back the failed item, retains previous successful items and stops later
ones. Overlapping shared state must be handled through one owner-scoped operation.
Removing the library entry retains applied copies. (Local workspace, 2026)

CLI removal requires successful preview output even when `--yes` supplies the
approval. A closed output stream stops the operation before vault mutation.
(Local workspace, 2026)

## References

- Local workspace (2026). [Library contracts](../internal/library/library.go),
  [application](../internal/manager/library.go), [CLI](../internal/manager/library_cli.go),
  [TUI](../internal/manager/library_tui.go), [integration tests](../internal/manager/library_test.go).
