# Installation

Use the macOS release bootstrap below, or build with your installed Go toolchain.
The source-build version is declared in `go.mod`. Setup supports a private payload
directory with an optional launcher symlink, or a direct binary directory.
It requires no administrator access.
(Project, 2026)

## Build and inspect

```sh
git clone https://github.com/zaRizk7/harness-ctl.git
cd harness-ctl
GOTOOLCHAIN=local make build
./bin/harness-ctl version
./bin/harness-ctl --help
```

## Private installation

Bind setup to the binary you built, preview the paths, then approve the
installation. The local checksum binds the preview to these bytes. It is not an
independent publisher signature. Keep the binary and launcher under your home.
(Project, 2026)

```sh
binary_sha256="$(shasum -a 256 ./bin/harness-ctl | awk '{print $1}')"
./bin/harness-ctl setup --binary "$PWD/bin/harness-ctl" \
  --sha256 "$binary_sha256" --link-dir "$HOME/.local/bin" --preview
./bin/harness-ctl setup --binary "$PWD/bin/harness-ctl" \
  --sha256 "$binary_sha256" --link-dir "$HOME/.local/bin"
```

The second command opens the setup TUI. Review the prefix and link mode, press
Enter for the preview and type `setup` to approve. Add `--headless` for a text
preview and approval prompt. Add `--yes` only when approving the printed concrete
setup without a prompt. Omit `--link-dir` for a direct installation. An existing
binary destination is refused. (Project, 2026)

## PATH and editable catalog

The default manager data directory is
`~/Library/Application Support/harness-ctl`. Setup creates its user-owned
`config.json` and `catalog.json` without overwriting existing edits. Managed
harness shims use the directory printed by `harness-ctl path`. (Project, 2026)

```sh
"$HOME/.local/bin/harness-ctl" path
"$HOME/.local/bin/harness-ctl" setup --shell-file "$HOME/.zshrc" --headless --preview
"$HOME/.local/bin/harness-ctl" setup --shell-file "$HOME/.zshrc" --headless
```

Start a new shell after approving the PATH change. The explicit shell file must
be inside your home and unlinked. Fish and other non-POSIX shells need their
native PATH syntax. Change `catalog.json` to customize supported metadata and
launch rules. Native install/format contracts remain adapter-owned.
(Project, 2026)

## HTTPS bootstrap

The bootstrap selects your architecture and checks the binary against the
selected release's SHA-256 metadata before running setup. It supports interactive
setup through `/dev/tty` and approved unattended setup. These commands require
published assets. [v0.1.1](https://github.com/zaRizk7/harness-ctl/releases/tag/v0.1.1)
is public with both macOS architectures. Actual latest/pinned headless and
interactive installation passed on Apple Silicon. Intel has native CI
quality/build/version proof. Binary installation does not require Go.
(Project, 2026)

```sh
curl -fsSL https://github.com/zaRizk7/harness-ctl/releases/latest/download/install.sh | sh
curl -fsSL https://github.com/zaRizk7/harness-ctl/releases/latest/download/install.sh \
  | sh -s -- --headless --yes
curl -fsSL https://github.com/zaRizk7/harness-ctl/releases/download/v0.1.1/install.sh \
  | sh -s -- --version v0.1.1
```

For inspection, download
[scripts/install.sh](https://github.com/zaRizk7/harness-ctl/blob/main/scripts/install.sh)
to a file before execution. Explicit HTTPS binary URLs and trusted SHA-256
overrides remain supported. The
[setup guide](https://github.com/zaRizk7/harness-ctl/blob/main/docs/setup.md) contains
prefix, direct/symlink, PATH and checksum options. Checksums use the publisher's
HTTPS metadata and are not independent signatures. (Project, 2026)

## References

- Project (2026). [Setup guide](https://github.com/zaRizk7/harness-ctl/blob/main/docs/setup.md),
  [setup contract](https://github.com/zaRizk7/harness-ctl/blob/main/internal/manager/setup.go),
  [Go module](https://github.com/zaRizk7/harness-ctl/blob/main/go.mod).
