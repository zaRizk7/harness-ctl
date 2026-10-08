# Component management

Select a harness, select its installation with Tab, then choose **Manage
components**. The categories are skills, MCP, plugins, connectors, proxies,
hooks, settings, memory and other local assets. Each row identifies its source
and affected owners. Configuration values appear only in your editor.
(Local workspace, 2026)

| Control | Result |
| --- | --- |
| Enter on a category | List entries in the selected source |
| `a` | Open an add/install request in `$VISUAL`, `$EDITOR`, or `vi` |
| `e` or Enter on an entry | Edit its JSON value or file content |
| Enter on a directory | Browse its files, including disabled files |
| `d`, `u`, Space | Disable, enable, or toggle the selected entry |
| `x` | Preview removal of the selected entry |
| `o`, then Space | Select each additional affected owner |
| `p` | Switch between base state and an existing launch profile |
| `r` | Refresh the component list after browsing a directory |
| Esc | Return to category selection, or leave management |

Every change produces a preview. Review its sources, destinations, commands and
owners, then type `apply` and press Enter. Closing the editor without changes
cancels the edit. Invalid requests return to the component list with an error.
The scope label shows the state used by the selected launch. If a profile is
active, management starts in that profile. (Local workspace, 2026)

## Add/install requests

The add editor contains one JSON request. Keep `operation` as `add` or `install`
and retain the selected `category`. Replace placeholders with actual paths,
names and vendor configuration. Unknown request properties are rejected.
The manager does not download an MCP executable merely because you register it.
(Local workspace, 2026)

| Property | Meaning |
| --- | --- |
| `path` | Absolute destination inside selected native state |
| `field` | JSON pointer to one configuration entry, independent of file format |
| `value` | JSON value for that entry, including entries in TOML/YAML files |
| `source` | Absolute local file/directory to import, or a native plugin source |
| `text` | File content when adding an asset without `source` |
| `native` | Use a verified native plugin command when `true` |
| `name` | Plugin identifier or expected Gemini extension name |
| `scope` | `base` or `profile`, fixed to the screen's current selection |

For a Codex MCP registration, replace the example destination with the path
shown by the selected installation. (Local workspace, 2026)

```json
{
  "operation": "add",
  "category": "mcp",
  "path": "/absolute/codex-state/config.toml",
  "field": "/mcp_servers/demo",
  "value": {"command": "/absolute/path/to/server", "args": [], "enabled": true}
}
```

Import a skill directory containing `SKILL.md` and its supporting files. Codex's
current user skill source is `$HOME/.agents/skills`. Base managed launches inherit
that shared source. Profile launches use `<profile>/home/.agents/skills`. Older
state-root skill assets can also appear in inventory. (OpenAI, 2026. Local
workspace, 2026)

```json
{
  "operation": "install",
  "category": "skills",
  "path": "/absolute/home/.agents/skills/demo",
  "source": "/absolute/source/demo"
}
```

Native Claude plugins use a qualified identifier from an already configured
marketplace. Gemini extensions use a repository URL or local directory, plus
the expected name from `gemini-extension.json`. The preview includes the exact
native command. (Anthropic, 2026. Google, 2026. Local workspace, 2026)

```json
{
  "operation": "install",
  "category": "plugins",
  "native": true,
  "source": "demo@my-marketplace"
}
```

```json
{
  "operation": "install",
  "category": "plugins",
  "native": true,
  "source": "/absolute/source/my-extension",
  "name": "my-extension"
}
```

For a plain local asset, supply `source` to copy a file/directory or `text` to
create a file. Structured JSON, TOML and YAML asset edits must parse before a
preview can be approved. Import sources and destinations cannot overlap or
contain symlinks. Executable file modes survive editing. (Local workspace, 2026)

## Capabilities and limits

| Harness | Management implemented |
| --- | --- |
| Codex CLI | Shared user skills, configuration entries, native `enabled` flags for MCP/app registrations, local assets |
| Claude Code | User MCP/settings entries, skills, native user-scope plugin install/enable/disable/remove, local assets |
| Gemini CLI | MCP/settings entries, skills, native user-scope extension install/enable/disable/remove, editable extension files |
| OpenCode | JSON MCP entries and their `enabled` flags, plugin array entries, local assets |
| Pi | Existing settings registrations and local skills/extensions/assets |
| Hermes Agent | Existing YAML registrations and local assets |
| OpenClaw | Existing JSON registrations and local assets |
| Prime Agent | Existing JSON/TOML registrations and local assets |

The table describes this manager's adapters. Local file management is available
for classified categories across all eight harnesses. A filename/category does
not establish that a vendor loads the file. Where no native add template exists,
the editor proposes a local asset. Supply the vendor's supported path/shape or
edit an existing registration. JSONC and opaque/database resources remain whole
assets. There is no universal vendor installer for every category.
(Local workspace, 2026)

Connector management covers local registrations and overrides. Adding an app ID
does not install an account connection. OAuth, account provisioning, revocation
and OS-held secrets remain vendor-managed. Proxy edits cover selected files and
configuration entries. Shell environment and system network proxies retain their
existing behavior. Project and system sources are outside these screens.
(Local workspace, 2026)

## Disable, remove and recover

Verified native flags or commands retain disabled configuration in its native
location. Other entries are parked outside the active loader source under
`.harness-ctl-disabled/<category>/<id>`. Metadata records the original location
and value. Asset payloads retain their files and modes. Array positions are
tracked so restoring entries in different orders preserves their order.
(Local workspace, 2026)

Profile/migration copies relocate parked records to their new scope. Native
Claude commands reject payload paths outside the selected captured state before
running. Copied vendor plugin ledgers may require a native reinstall in the new
scope. Gemini commands require the selected root to be a `.gemini` directory
under its launch HOME. (Local workspace, 2026)

Parked state is ordinary local state with private parent-directory permissions,
not an encrypted archive. It remains until enabled or removed. Lifecycle
preservation/discard policies include parked entries in their original category.
Shared Codex skills also require selecting other agents using that source.
Removal retains encrypted recovery for the configured retention period. Use
the main Recovery screen to restore it. (Local workspace, 2026)

The transaction engine checks ownership and stale sources under its mutation
lock, captures encrypted recovery, applies the change and verifies the result.
Unrelated local state must remain unchanged. Failed or cancelled operations
restore captured files, or leave a recovery journal if restoration fails.
(Local workspace, 2026)

Native installers can download and execute upstream code. Verification checks
registrations, manifests and retained settings, rather than accepting only the
exit status. Recovery covers captured local files. Upstream scripts, OS secret
stores and external side effects cannot be reversed by those snapshots. Vendor
version compatibility, live account connections and real native installers have
not been exercised by this repository's synthetic tests. (Local workspace, 2026)

## References

- Local workspace (2026). [Component engine](../internal/manager/components.go),
  [native adapters](../internal/manager/components_native.go),
  [TUI](../internal/manager/components_tui.go),
  [component tests](../internal/manager/components_test.go),
  [TUI tests](../internal/manager/tui_test.go).
- OpenAI (2026). [Skills](https://learn.chatgpt.com/docs/build-skills),
  [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).
- Anthropic (2026). [Plugin commands](https://code.claude.com/docs/en/plugins/cli-reference).
- Google (2026). [Extension reference](https://geminicli.com/docs/extensions/reference/),
  [enablement implementation](https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/extensions/extensionEnablement.ts).
- OpenCode (2026). [MCP servers](https://opencode.ai/docs/mcp-servers/).
