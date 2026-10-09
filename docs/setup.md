# Installation and user configuration

Use a private user prefix with a launcher symlink in `~/.local/bin`. This keeps
executable payloads separate from editable configuration, needs no administrator
access and makes removal explicit. A direct binary directory is also supported.
Shell startup changes require an explicit `--shell-file` path and approval. (Local workspace, 2026)

The authorized destination is [zaRizk7/harness-ctl](https://github.com/zaRizk7/harness-ctl).
The bootstrap selects Apple Silicon or Intel and verifies the release binary
against its SHA-256 entry before running setup. The default resolves the latest
published release. Pin a version for repeatable installation. Release metadata
comes from the same publisher over HTTPS, so a checksum detects changed bytes
but does not provide an independent publisher signature. (Local workspace, 2026)

```sh
curl -fsSL https://github.com/zaRizk7/harness-ctl/releases/latest/download/install.sh | sh
# Unattended setup. Existing destinations are still refused.
curl -fsSL https://github.com/zaRizk7/harness-ctl/releases/latest/download/install.sh \
  | sh -s -- --headless --yes
# Pin both installer and binary release.
curl -fsSL https://github.com/zaRizk7/harness-ctl/releases/download/v0.1.0/install.sh \
  | sh -s -- --version v0.1.0
```

The pinned examples use `v0.1.0`. Check [Releases](https://github.com/zaRizk7/harness-ctl/releases)
and the [latest publication checkpoint](https://github.com/zaRizk7/harness-ctl/blob/main/docs/implementation.md)
for availability and published-asset verification. These commands require
published release assets. The installer stops on absent
assets, missing/duplicate checksum entries, wrong architecture or mismatched
bytes. Interactive setup reopens `/dev/tty`, so the setup TUI can receive input
when installation is piped into `sh`. Unattended use requires both `--headless`
and `--yes`. The installer does not install Go or use administrator access.
(Local workspace, 2026)

For inspection before execution, download the installer instead. A trusted
explicit checksum and custom HTTPS binary URL remain supported. (Local workspace, 2026)

```sh
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/zaRizk7/harness-ctl/main/scripts/install.sh
# Inspect install.sh before running it.
sh install.sh --version v0.1.0 --prefix "$HOME/.local/lib/harness-ctl" \
  --link-dir "$HOME/.local/bin"
sh install.sh --url BINARY_HTTPS_URL --sha256 PUBLISHER_SHA256 --direct
sh install.sh --version v0.1.0 --shell-file "$HOME/.zshrc" --headless
```

`HARNESS_CTL_VERSION` selects the release. `HARNESS_CTL_REPOSITORY` selects a
reviewed `owner/repository` mirror with the same asset names.
`HARNESS_CTL_DOWNLOAD_SECONDS` bounds each download and defaults to 120 seconds.
Explicit binary/checksum environment overrides are `HARNESS_CTL_BINARY_URL` and
`HARNESS_CTL_SHA256`. Changes to a mirror select a different publisher trust
boundary. No existing catalog or configuration is overwritten during setup.
(Local workspace, 2026)

## Source installation

```sh
git clone https://github.com/zaRizk7/harness-ctl.git
cd harness-ctl
GOTOOLCHAIN=local make build
binary_sha256="$(shasum -a 256 ./bin/harness-ctl | awk '{print $1}')"
./bin/harness-ctl setup --binary "$PWD/bin/harness-ctl" \
  --sha256 "$binary_sha256" --link-dir "$HOME/.local/bin" --preview
./bin/harness-ctl setup --binary "$PWD/bin/harness-ctl" \
  --sha256 "$binary_sha256" --link-dir "$HOME/.local/bin"
```

The checksum binds setup to your local build, rather than proving independent
publisher authenticity. `setup` without `--binary` initializes configuration
and optional PATH only. Supply a binary and checksum to install its payload.
(Local workspace, 2026)

The setup TUI displays the executable prefix and symlink choice. Space switches
between direct and symlink installation, Enter builds a concrete preview, and
`setup` approves the listed paths. Use flags for custom directories. Existing
executable destinations are refused. Uninstall the manager while retaining state
before reinstalling into the same destination. (Local workspace, 2026)

With an already built or downloaded binary:

```sh
harness-ctl setup
harness-ctl setup --headless --yes
harness-ctl setup --binary /absolute/download/harness-ctl \
  --sha256 PUBLISHER_SHA256 --prefix /absolute/user-prefix \
  --link-dir /absolute/user-bin --headless
harness-ctl setup --shell-file /absolute/home/.zshrc --headless --preview
harness-ctl setup --shell-file /absolute/home/.zshrc --headless
harness-ctl setup --preview
harness-ctl path
```

Explicit setup initializes `<root>/config.json` and `<root>/catalog.json` without
overwriting existing edits. Startup reads these user-owned files without creating
manager storage. Embedded catalog data bootstraps new installations. An explicit
`--config` selects another configuration, and `catalog_file` selects another
catalog. Invalid catalogs and linked ownership paths fail before discovery.
Configuration and executable prefixes must remain inside the selected user home.
(Local workspace, 2026)

The optional symlink and binary location are recorded in `installation.json`.
Self-uninstall verifies that launcher still points to the selected binary before
removing it. Other commands, harness shims and user state remain governed by the
separate self-removal choices. (Local workspace, 2026)

## PATH activation

`--shell-file ABSOLUTE_FILE` appends a reviewed POSIX PATH assignment for the
harness shim directory and the selected manager launcher/direct binary directory.
The bootstrap forwards the same option. It must be a regular, unlinked file
inside the selected home, or an absent path with safe ancestors. Setup preserves
existing bytes and mode, fingerprints the preview, avoids identical duplicate
entries and restores the original file on failure. It does not source the file
into your current shell. Activate the PATH there, or start a new shell. Fish and
other non-POSIX startup syntax require their native PATH configuration.
(Local workspace, 2026)

## References

- Local workspace (2026). [Setup](../internal/setup/setup.go),
  [setup CLI](../internal/manager/setup.go), [installer](../scripts/install.sh),
  [setup tests](../internal/setup/setup_test.go), [installer tests](../internal/setup/installer_test.go).
- User decisions (2026), recorded in [the checkpoint](implementation.md).
