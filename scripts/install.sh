#!/bin/sh
# Install the latest tome release:
#   curl -fsSL https://raw.githubusercontent.com/huyphung1602/tome/main/scripts/install.sh | sh
# Options (env): TOME_VERSION=v0.1.0  TOME_INSTALL_DIR=~/.local/bin
set -eu

REPO=${TOME_REPO:-huyphung1602/tome}
DIR=${TOME_INSTALL_DIR:-$HOME/.local/bin}

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
FreeBSD) os=freebsd ;;
*) echo "unsupported OS: $(uname -s); download a release manually from https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

version=${TOME_VERSION:-$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
	sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)}
[ -n "$version" ] || { echo "could not find the latest release of $REPO" >&2; exit 1; }

name="tome_${version#v}_${os}_${arch}"
url="https://github.com/$REPO/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading $name.tar.gz"
curl -fsSL -o "$tmp/$name.tar.gz" "$url/$name.tar.gz"
curl -fsSL -o "$tmp/checksums.txt" "$url/checksums.txt"
(
	cd "$tmp"
	grep " $name.tar.gz\$" checksums.txt >want.txt
	if command -v sha256sum >/dev/null; then sha256sum -c want.txt; else shasum -a 256 -c want.txt; fi
) >/dev/null || { echo "checksum mismatch" >&2; exit 1; }

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
mkdir -p "$DIR"
install -m 0755 "$tmp/$name/tome" "$DIR/tome"
echo "installed tome $version to $DIR/tome"
case ":$PATH:" in *":$DIR:"*) ;; *) echo "note: $DIR is not on your PATH" ;; esac
