#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
shasum -a 256 -c SHA256SUMS
output=Aheadscale-macOS-ARM64-SIGNED.pkg
receipt=notarization-pkg.json
notary_profile=${AHEADSCALE_NOTARY_PROFILE:-Aheadscale}
if [[ -f "$output" ]]; then
    if [[ -f "$receipt" && "$(plutil -extract status raw -o - "$receipt")" = Accepted ]]; then
        xcrun stapler staple "$output"
        xcrun stapler validate "$output"
        pkgutil --check-signature "$output"
        spctl --assess --type install --verbose=2 "$output"
        shasum -a 256 "$output" > SIGNED-PKG-SHA256SUMS
        exit 0
    fi
    echo 'Пакет уже подписан. Проверьте существующую заявку Apple перед новой отправкой.' >&2
    exit 1
fi
work=$(mktemp -d "${TMPDIR:-/tmp}/aheadscale-pkg-sign.XXXXXX")
trap 'rm -rf "$work"' EXIT
for f in payload scripts package.sh package-version.txt app.entitlements provider.entitlements bridge.entitlements; do
    ditto "$f" "$work/$f"
done
app="$work/payload/Applications/Aheadscale.app"
provider="$app/Contents/Library/SystemExtensions/ru.aheadscale.client.tunnel.systemextension"
gui="$app/Contents/Library/LoginItems/Aheadscale UI.app"
bridge="$app/Contents/Library/LaunchServices/AheadscaleNativeEngineBridge"
engine="$work/payload/Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine"
application_identity=${AHEADSCALE_APPLICATION_IDENTITY:-215518B86C54EAA83E94C4E27069A30B2D18D4D3}
installer_identity=${AHEADSCALE_INSTALLER_IDENTITY:-5D7567DCB1222363A7B826BDCA93958ED3369F47}
cp "${AHEADSCALE_APP_PROFILE:-$HOME/Downloads/Aheadscale.provisionprofile}" "$app/Contents/embedded.provisionprofile"
cp "${AHEADSCALE_TUNNEL_PROFILE:-$HOME/Downloads/Aheadscale_Tunnel.provisionprofile}" "$provider/Contents/embedded.provisionprofile"
sign() { codesign --force --options runtime --timestamp --sign "$application_identity" "$@"; }
sign --identifier ru.aheadscale.client.engine "$engine"
sign --identifier ru.aheadscale.client.cli "$app/Contents/MacOS/aheadscale"
sign --entitlements "$work/bridge.entitlements" --identifier ru.aheadscale.client.bridge "$bridge"
sign --entitlements "$work/provider.entitlements" "$provider"
sign "$gui"
sign --entitlements "$work/app.entitlements" "$app"
codesign --verify --deep --strict "$app"
codesign --verify --strict "$engine"
bash "$work/package.sh" "$work/unsigned.pkg"
productsign --sign "$installer_identity" "$work/unsigned.pkg" "$output"
pkgutil --check-signature "$output"
xcrun notarytool submit "$output" --keychain-profile "$notary_profile" --wait --timeout 10m --output-format json > "$receipt"
plutil -extract id raw -o - "$receipt"
test "$(plutil -extract status raw -o - "$receipt")" = Accepted
rm -rf "$work"
trap - EXIT
exec bash ./sign-on-macbook.sh
