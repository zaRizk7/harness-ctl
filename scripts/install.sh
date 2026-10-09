#!/bin/bash
# Install a publisher-verified release into private user storage. No sudo or Go.
set -euo pipefail
umask 077
url="${HARNESS_CTL_BINARY_URL:-}"
sha256="${HARNESS_CTL_SHA256:-}"
root=""
prefix=""
link_dir="$HOME/.local/bin"
headless=0
approve=0
shell_file=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --url) url="$2"; shift 2 ;;
    --sha256) sha256="$2"; shift 2 ;;
    --root) root="$2"; shift 2 ;;
    --prefix) prefix="$2"; shift 2 ;;
    --link-dir) link_dir="$2"; shift 2 ;;
    --direct) link_dir=""; shift ;;
    --headless) headless=1; shift ;;
    --yes) approve=1; shift ;;
    --shell-file) shell_file="$2"; shift 2 ;;
    --help) printf '%s\n' 'install.sh --url HTTPS_BINARY_URL --sha256 PUBLISHER_HASH [--root DIR] [--prefix DIR] [--link-dir DIR|--direct] [--shell-file ABSOLUTE_FILE] [--headless] [--yes]'; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done
if [ "$(uname -s)" != Darwin ]; then printf '%s\n' 'macOS is required' >&2; exit 1; fi
case "$(uname -m)" in arm64|x86_64) ;; *) printf '%s\n' 'Unsupported architecture' >&2; exit 1 ;; esac
case "$url" in https://*) ;; *) printf '%s\n' 'A release HTTPS binary URL is required' >&2; exit 2 ;; esac
if [[ ! "$sha256" =~ ^[a-f0-9]{64}$ ]]; then printf '%s\n' 'Supply the publisher SHA-256 before downloading or executing a binary' >&2; exit 2; fi
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/harness-ctl-install.XXXXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
binary="$tmp_dir/harness-ctl"
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 --output "$binary" "$url"
read -r actual _ < <(shasum -a 256 "$binary")
if [ "$actual" != "$sha256" ]; then printf '%s\n' 'SHA-256 mismatch. Installation stopped.' >&2; exit 1; fi
chmod 700 "$binary"
args=()
if [ -n "$root" ]; then args+=(--root "$root"); fi
args+=(setup --binary "$binary" --sha256 "$sha256")
if [ -n "$prefix" ]; then args+=(--prefix "$prefix"); fi
if [ -n "$link_dir" ]; then args+=(--link-dir "$link_dir"); fi
if [ "$headless" = 1 ]; then args+=(--headless); fi
if [ "$approve" = 1 ]; then args+=(--yes); fi
if [ -n "$shell_file" ]; then args+=(--shell-file "$shell_file"); fi
"$binary" "${args[@]}"
printf '%s\n' 'Activate the displayed PATH in your current shell. Startup files change only when --shell-file is selected and approved.'
