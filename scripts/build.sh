#!/bin/sh
# Build release archives for every supported platform into dist/.
#
#   scripts/build.sh                  # version from `git describe`
#   VERSION=v0.1.0 scripts/build.sh
#   TARGETS="darwin/arm64 linux/amd64" scripts/build.sh
set -eu

cd "$(dirname "$0")/.."

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
TARGETS=${TARGETS:-"darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 freebsd/amd64"}
DIST=dist

rm -rf "$DIST"
mkdir -p "$DIST"

for target in $TARGETS; do
	os=${target%/*}
	arch=${target#*/}
	name="tome_${VERSION#v}_${os}_${arch}"
	stage="$DIST/$name"
	exe=tome
	[ "$os" = windows ] && exe=tome.exe

	echo "building $name"
	mkdir -p "$stage"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$VERSION" \
		-o "$stage/$exe" ./cmd/tome
	cp README.md "$stage/"
	[ -f LICENSE ] && cp LICENSE "$stage/"

	if [ "$os" = windows ]; then
		(cd "$DIST" && zip -qr "$name.zip" "$name")
	else
		tar -C "$DIST" -czf "$DIST/$name.tar.gz" "$name"
	fi
	rm -rf "$stage"
done

cd "$DIST"
if command -v sha256sum >/dev/null; then
	sha256sum -- *.tar.gz *.zip >checksums.txt
else
	shasum -a 256 -- *.tar.gz *.zip >checksums.txt
fi
echo "done: $(pwd)"
ls -1
