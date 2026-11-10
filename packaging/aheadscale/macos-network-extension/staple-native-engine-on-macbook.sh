#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
[ "$(uname -s)" = Darwin ] || exit 1
shasum -a 256 -c SIGNED_SHA256SUMS
work=$(mktemp -d "${TMPDIR:-/tmp}/aheadscale-native-staple.XXXXXX")
trap 'rm -rf "$work"' EXIT
ditto -x -k AheadscaleNativeEngine-SIGNED.zip "$work"
app="$work/Aheadscale Extension Setup.app"
codesign --verify --deep --strict -R '=anchor apple generic and certificate leaf[subject.OU] = "Y24464MUL3" and identifier "ru.aheadscale.client"' "$app"
xcrun stapler staple "$app"
xcrun stapler validate "$app"
spctl --assess --type execute --verbose=2 "$app"
ditto -c -k --keepParent "$app" AheadscaleNativeEngine-STAPLED.zip
scp AheadscaleNativeEngine-STAPLED.zip adnako@macmini.tailb71cbe.ts.net:/tmp/aheadscale-macos-native-engine-signing/
echo 'Полный native-engine комплект с билетом Apple проверен и передан. Ничего не устанавливалось.'
