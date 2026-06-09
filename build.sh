#!/usr/bin/env bash
# Cross-compile the relay for every desktop/Pi target into ./dist/.
# These are the files attached to a GitHub Release (see RELEASE.md) and named
# exactly as the kibitz.chat download page expects.
#
#   ./build.sh                      # bakes the default site (kibitz.chat)
#   BASE=https://example.com ./build.sh   # rebrand for a fork
set -euo pipefail
cd "$(dirname "$0")"

BASE="${BASE:-}"                     # empty = use the in-source DefaultLinkBase
LDFLAGS="-s -w"                      # strip symbols → smaller binaries
[ -n "$BASE" ] && LDFLAGS="$LDFLAGS -X kibitz-relay/relaycore.DefaultLinkBase=$BASE"

mkdir -p dist

build() { # os arch outname
  echo "  $3"
  GOOS="$1" GOARCH="$2" CGO_ENABLED=0 \
    go build -trimpath -ldflags "$LDFLAGS" -o "dist/$3" .
}

echo "building relay binaries${BASE:+ (base $BASE)} …"
build windows amd64 kibitz-relay-windows.exe
build darwin  arm64 kibitz-relay-mac-apple-silicon
build darwin  amd64 kibitz-relay-mac-intel
build linux   amd64 kibitz-relay-linux
build linux   arm64 kibitz-relay-raspberry-pi

# The Pi appliance kit travels with the release so the systemd guide is one click away.
cp pi/kibitz-relay.service pi/README.md dist/

echo "done → dist/"
ls -lh dist/
