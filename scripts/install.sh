#!/bin/sh
# Verify a release before running setup in private user storage. No sudo or Go.
set -eu
umask 077
url=${HARNESS_CTL_BINARY_URL:-}
sha256=${HARNESS_CTL_SHA256:-}
version=${HARNESS_CTL_VERSION:-latest}
repository=${HARNESS_CTL_REPOSITORY:-zaRizk7/harness-ctl}
root=
prefix=
link_dir="$HOME/.local/bin"
headless=0
approve=0
shell_file=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --url|--sha256|--version|--root|--prefix|--link-dir|--shell-file)
      [ "$#" -ge 2 ] || { printf 'Missing value: %s\n' "$1" >&2; exit 2; }
      case "$1" in
        --url) url=$2 ;; --sha256) sha256=$2 ;; --version) version=$2 ;;
        --root) root=$2 ;; --prefix) prefix=$2 ;; --link-dir) link_dir=$2 ;;
        --shell-file) shell_file=$2 ;;
      esac
      shift 2 ;;
    --direct) link_dir=; shift ;;
    --headless) headless=1; shift ;;
    --yes) approve=1; shift ;;
    --help) printf '%s\n' 'install.sh [--version TAG] [--url HTTPS_BINARY_URL --sha256 PUBLISHER_HASH] [--root DIR] [--prefix DIR] [--link-dir DIR|--direct] [--shell-file ABSOLUTE_FILE] [--headless] [--yes]'; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done
[ "$(uname -s)" = Darwin ] || { printf '%s\n' 'macOS is required' >&2; exit 1; }
case "$(uname -m)" in
  arm64) arch=arm64 ;; x86_64) arch=amd64 ;;
  *) printf '%s\n' 'Unsupported architecture' >&2; exit 1 ;;
esac
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/harness-ctl-install.XXXXXXXX")
trap 'rm -rf "$tmp_dir"' 0
trap 'exit 1' HUP INT TERM
# HTTPS is enforced on every redirect. Failed or absent releases stop installation.
download() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 \
    --max-time "${HARNESS_CTL_DOWNLOAD_SECONDS:-120}" --output "$2" "$1"
}
if [ -z "$url" ]; then
  case "$repository" in
    *[!A-Za-z0-9_./-]*|/*|*..*|*/|*/*/*) printf '%s\n' 'Invalid release repository' >&2; exit 2 ;;
    */*) ;; *) printf '%s\n' 'Use owner/repository' >&2; exit 2 ;;
  esac
  case "$version" in
    latest) base="https://github.com/$repository/releases/latest/download" ;;
    v[0-9]*.[0-9]*.[0-9]*)
      case "$version" in *[!A-Za-z0-9.-]*) printf '%s\n' 'Invalid release tag' >&2; exit 2 ;; esac
      base="https://github.com/$repository/releases/download/$version" ;;
    *) printf '%s\n' 'Use latest or a vMAJOR.MINOR.PATCH release tag' >&2; exit 2 ;;
  esac
  asset="harness-ctl-darwin-$arch"
  url="$base/$asset"
  if [ -z "$sha256" ]; then
    download "$base/SHA256SUMS" "$tmp_dir/SHA256SUMS"
    sha256=$(awk -v asset="$asset" '$2 == asset { sum=$1; count++ } END { if (count != 1) exit 1; print sum }' "$tmp_dir/SHA256SUMS") || {
      printf '%s\n' 'Release checksum entry is absent or ambiguous' >&2; exit 1;
    }
  fi
fi
case "$url" in https://*) ;; *) printf '%s\n' 'A release HTTPS binary URL is required' >&2; exit 2 ;; esac
case "$sha256" in
  *[!a-f0-9]*|'') printf '%s\n' 'Supply a lowercase publisher SHA-256' >&2; exit 2 ;;
esac
[ "${#sha256}" = 64 ] || { printf '%s\n' 'SHA-256 must contain 64 hexadecimal characters' >&2; exit 2; }
binary="$tmp_dir/harness-ctl"
download "$url" "$binary"
actual=$(shasum -a 256 "$binary")
actual=${actual%% *}
[ "$actual" = "$sha256" ] || { printf '%s\n' 'SHA-256 mismatch. Installation stopped.' >&2; exit 1; }
chmod 700 "$binary"
set -- setup --binary "$binary" --sha256 "$sha256"
[ -z "$root" ] || set -- --root "$root" "$@"
[ -z "$prefix" ] || set -- "$@" --prefix "$prefix"
[ -z "$link_dir" ] || set -- "$@" --link-dir "$link_dir"
[ "$headless" = 0 ] || set -- "$@" --headless
[ "$approve" = 0 ] || set -- "$@" --yes
[ -z "$shell_file" ] || set -- "$@" --shell-file "$shell_file"
# Piped shell input contains this script, not terminal input. Reopen the TTY for
# interactive setup/approval. Unattended execution requires explicit approval.
if [ "$headless" = 1 ] && [ "$approve" = 1 ]; then
  "$binary" "$@" </dev/null
elif ( : </dev/tty ) 2>/dev/null; then
  "$binary" "$@" </dev/tty
else
  printf '%s\n' 'Setup needs a terminal. For unattended setup use --headless --yes.' >&2
  exit 2
fi
printf '%s\n' 'Activate the displayed PATH in your current shell. Startup files change only when --shell-file is selected and approved.'
