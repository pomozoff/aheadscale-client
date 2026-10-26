#!/usr/bin/env bash
set -euo pipefail
platform=${1:?platform: linux, windows or darwin}
arch=${2:?architecture: amd64 or arm64}
channel=${3:?channel: internal or public}
case "$channel" in
  internal) origin=https://macmini.tailb71cbe.ts.net:10443 ;;
  public) origin=https://aheadscale.ru:443 ;;
  *) echo 'Invalid build channel' >&2; exit 1 ;;
esac
case "$platform:$arch" in
  linux:amd64|linux:arm64|windows:amd64|windows:arm64|darwin:amd64|darwin:arm64) ;;
  *) echo 'Invalid build target' >&2; exit 1 ;;
esac
export GOOS=$platform GOARCH=$arch GOTOOLCHAIN=local
if [[ $platform == darwin ]]; then
  export MACOSX_DEPLOYMENT_TARGET=11.0 MACOS_DEPLOYMENT_TARGET=11.0
fi
out="$PWD/build/$platform-$arch-$channel"
mkdir -p "$out"
extension=''
if [[ $platform == windows ]]; then extension=.exe; fi
ldflags="-s -w -X github.com/netbirdio/netbird/version.version=0.80.0-aheadscale.22 -X github.com/netbirdio/netbird/client/internal/profilemanager.DefaultManagementURL=$origin -X github.com/netbirdio/netbird/client/internal/profilemanager.DefaultAdminURL=$origin -X github.com/netbirdio/netbird/client/internal/profilemanager.LockedServerOrigin=$origin"
CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$out/aheadscale$extension" ./client
export CGO_ENABLED=1
export VITE_AHEADSCALE_MANAGEMENT_URL=$origin
pushd client/ui >/dev/null
GOOS=$(go env GOHOSTOS) GOARCH=$(go env GOHOSTARCH) CC=cc CXX=c++ wails3 generate bindings -clean=true -ts
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend build
if [[ $platform == linux && $channel == internal ]]; then pnpm --dir frontend lint; pnpm --dir frontend typecheck; fi
popd >/dev/null
ui_ldflags=$ldflags
if [[ $platform == windows ]]; then
  ui_ldflags="$ui_ldflags -H windowsgui"
  syso="$PWD/client/ui/wails_windows_${arch}.syso"
  trap 'rm -f "$syso"' EXIT
  wails3 generate syso -arch "$arch" -icon packaging/aheadscale/icon.ico \
    -manifest client/ui/build/windows/wails.exe.manifest \
    -info client/ui/build/windows/info.json -out "$syso"
fi
go build -trimpath -tags production -ldflags "$ui_ldflags" -o "$out/aheadscale-ui$extension" ./client/ui
python3 packaging/aheadscale/package.py --platform "$platform" --arch "$arch" --channel "$channel"
if [[ $platform == linux ]]; then
  python3 packaging/aheadscale/package.py --platform "$platform" --arch "$arch" --channel "$channel" --cli-only
fi
