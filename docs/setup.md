# Installation and user configuration

Use a private user prefix with a launcher symlink in `~/.local/bin`. This keeps
executable payloads separate from editable configuration, needs no administrator
access and makes removal explicit. A direct binary directory is also supported.
Shell startup changes require an explicit `--shell-file` path and approval. (Local workspace, 2026)

The authorized destination is [zaRizk7/harness-ctl](https://github.com/zaRizk7/harness-ctl).
No binary release has been published yet. Build from source today, or use the
bootstrap after an approved release supplies assets and checksums. The installer
requires an explicit HTTPS binary URL and publisher SHA-256.
Download the installer to a file, inspect it, then run it. The installer verifies
the binary before executing its setup interface. Obtain the checksum through a
trusted publisher channel. Pin a reviewed commit for the installer URL when
reproducibility is needed. (User decisions, 2026; Local workspace, 2026)

```sh
curl --fail --location --proto '=https' --proto-redir '=https' --tlsv1.2 \
  -o install.sh https://raw.githubusercontent.com/zaRizk7/harness-ctl/main/scripts/install.sh
# Inspect install.sh before running it.
bash install.sh --url BINARY_HTTPS_URL --sha256 PUBLISHER_SHA256
bash install.sh --url BINARY_HTTPS_URL --sha256 PUBLISHER_SHA256 --headless --yes
bash install.sh --url BINARY_HTTPS_URL --sha256 PUBLISHER_SHA256 --direct
```

The binary URL convention after a release is
`https://github.com/zaRizk7/harness-ctl/releases/download/TAG/harness-ctl-darwin-ARCH`,
with `ARCH` equal to `arm64` or `amd64` and an approved version tag. Obtain the
matching entry from that release's `SHA256SUMS` through a trusted publisher
channel. These are conventions for future releases, not currently downloadable
assets. (Local workspace, 2026)

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
