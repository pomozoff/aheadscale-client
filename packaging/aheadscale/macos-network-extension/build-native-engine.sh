#!/bin/bash
set -euo pipefail
output=${1:?Укажи путь выходного daemon}
[ "$(uname -s)" = Darwin ] || { echo 'Нужен macOS runner.' >&2; exit 1; }
origin=https://aheadscale.ru:443
version=0.80.0-aheadscale.native.1
export MACOSX_DEPLOYMENT_TARGET=13.0
flags="-s -w -X github.com/netbirdio/netbird/version.version=$version -X github.com/netbirdio/netbird/client/internal/profilemanager.DefaultManagementURL=$origin -X github.com/netbirdio/netbird/client/internal/profilemanager.DefaultAdminURL=$origin -X github.com/netbirdio/netbird/client/internal/profilemanager.LockedServerOrigin=$origin"
go build -trimpath -tags macosvpnprototype -ldflags "$flags" -o "$output" ./client
# Без обращения к серверу: проверка встроенного адреса после линковки.
"$output" up --help | grep -F "$origin" > /dev/null
