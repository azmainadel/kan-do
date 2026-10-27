#!/bin/sh
# Installs kando, the kan-do board, on macOS or Linux:
#   curl -fsSL https://github.com/azmainadel/kan-do/releases/latest/download/install.sh | sh
#
# KANDO_INSTALL_DIR  where to put kando (default ~/.local/bin)
# KANDO_VERSION      a version to install, like 0.1.0 (default the latest)
set -eu

repo="azmainadel/kan-do"
dir="${KANDO_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "kando install: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "needs $1"; }
need curl
need tar

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "kando runs on macOS and Linux" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  armv7* | armv8l) arch=armv7 ;;
  *) fail "no kando build for $(uname -m)" ;;
esac

version="${KANDO_VERSION:-}"
if [ -z "$version" ]; then
  # The latest release page redirects to its tag, like .../tag/v0.1.0.
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
    fail "cannot reach GitHub"
  version="${url##*/v}"
fi
version="${version#v}"
base="${KANDO_DOWNLOAD_URL:-https://github.com/$repo/releases/download/v$version}"
archive="kando_${version}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading kando $version for $os/${arch}…"
curl -fsSL -o "$tmp/$archive" "$base/$archive" || fail "cannot download $archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "cannot download checksums.txt"

want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || fail "checksums.txt has no line for $archive"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
else
  got=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
fi
[ "$got" = "$want" ] || fail "$archive does not match its checksum"

tar -xzf "$tmp/$archive" -C "$tmp" kando
mkdir -p "$dir"
mv "$tmp/kando" "$dir/kando"
chmod 755 "$dir/kando"
echo "Installed kando $version to $dir/kando"

case ":$PATH:" in
  *":$dir:"*) echo "Run: kando" ;;
  *) echo "Add $dir to your PATH, then run: kando" ;;
esac
