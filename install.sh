#!/bin/sh
# Install submarine (and the `sub` alias).
#   curl -fsSL https://raw.githubusercontent.com/simonfranken/submarine/main/install.sh | sh
#
# Environment:
#   SUBMARINE_VERSION      install this version (default: latest)
#   SUBMARINE_INSTALL_DIR  target directory (default: ~/.local/bin)
#   SUBMARINE_BASE_URL     releases root (default: the GitHub releases)
set -eu

base="${SUBMARINE_BASE_URL:-https://github.com/simonfranken/submarine/releases}"
dir="${SUBMARINE_INSTALL_DIR:-$HOME/.local/bin}"

die() { echo "install: $*" >&2; exit 1; }

case "$(uname -s)" in
Linux)  os=linux ;;
Darwin) os=darwin ;;
*) die "unsupported OS: $(uname -s) (linux and macOS only)" ;;
esac
case "$(uname -m)" in
x86_64|amd64)  arch=amd64 ;;
arm64|aarch64) arch=arm64 ;;
*) die "unsupported architecture: $(uname -m)" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required"
if command -v sha256sum >/dev/null 2>&1; then
	sha() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	die "sha256sum or shasum is required"
fi

version="${SUBMARINE_VERSION:-}"
if [ -z "$version" ]; then
	version="$(curl -fsSL "$base/latest/download/VERSION")" || die "cannot determine the latest version"
	version="$(printf '%s' "$version" | tr -d '[:space:]')"
fi
case "$version" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*) die "invalid version: $version" ;;
esac

asset="submarine_${os}_${arch}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Installing submarine $version ($os/$arch) to $dir"
curl -fsSL -o "$tmp/$asset" "$base/download/$version/$asset" || die "download failed: $asset"
curl -fsSL -o "$tmp/checksums.txt" "$base/download/$version/checksums.txt" || die "download failed: checksums.txt"

want="$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || die "no checksum for $asset"
[ "$(sha "$tmp/$asset")" = "$want" ] || die "checksum mismatch for $asset"

mkdir -p "$dir"
chmod +x "$tmp/$asset"
# Stage next to the target, then rename: atomic, and safe if it is running.
cp "$tmp/$asset" "$dir/.submarine.new"
mv -f "$dir/.submarine.new" "$dir/submarine"

if [ -e "$dir/sub" ] && [ "$(readlink "$dir/sub" 2>/dev/null)" != "submarine" ]; then
	echo "install: $dir/sub exists and is not ours, leaving it alone (use \`submarine\`)" >&2
else
	ln -sf submarine "$dir/sub"
fi

echo "Installed: $dir/submarine (alias: sub)"
case ":$PATH:" in
*":$dir:"*) ;;
*) echo "Note: $dir is not on your PATH. Add it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
echo "Update later with: submarine update"
