#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
[ "$(uname -s)" = Darwin ] || exit 1
shasum -a 256 -c SIGNING_SHA256SUMS
if [ -e AheadscaleNativeEngine-SIGNED.zip ]; then
  if [ -e notarization.json ] && [ "$(plutil -extract status raw -o - notarization.json 2>/dev/null || true)" = Accepted ]; then
    exec bash staple-native-engine-on-macbook.sh
  fi
  echo 'Подписанный архив уже есть. Не отправляю повторно; сначала проверь существующую заявку Apple.' >&2
  exit 1
fi
work=$(mktemp -d "${TMPDIR:-/tmp}/aheadscale-native-sign.XXXXXX")
trap 'rm -rf "$work"' EXIT
ditto -x -k AheadscaleExtensionSetup-UNSIGNED-PROTOTYPE.zip "$work"
app="$work/Aheadscale Extension Setup.app"
provider="$app/Contents/Library/SystemExtensions/ru.aheadscale.client.tunnel.systemextension"
broker="$app/Contents/Library/LaunchServices/AheadscaleNativeEngineBridge"
engine="$app/Contents/Library/LaunchServices/aheadscale-native"
cp /Users/apomozov/Downloads/Aheadscale.provisionprofile "$app/Contents/embedded.provisionprofile"
cp /Users/apomozov/Downloads/Aheadscale_Tunnel.provisionprofile "$provider/Contents/embedded.provisionprofile"
identity=215518B86C54EAA83E94C4E27069A30B2D18D4D3
codesign --force --options runtime --timestamp --identifier ru.aheadscale.client.engine --sign "$identity" "$engine"
codesign --force --options runtime --timestamp --identifier ru.aheadscale.client.bridge --sign "$identity" --entitlements root.sign.entitlements "$broker"
codesign --force --options runtime --timestamp --sign "$identity" --entitlements provider.sign.entitlements "$provider"
codesign --force --options runtime --timestamp --sign "$identity" --entitlements app.sign.entitlements "$app"
codesign --verify --deep --strict "$app"
for pair in 'ru.aheadscale.client.engine|aheadscale-native' 'ru.aheadscale.client.bridge|AheadscaleNativeEngineBridge'; do
  identifier=${pair%%|*}; executable=${pair#*|}
  codesign --verify --strict -R "=anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"$identifier\"" "$app/Contents/Library/LaunchServices/$executable"
done
codesign --verify --strict -R '=anchor apple generic and certificate leaf[subject.OU] = "Y24464MUL3" and identifier "ru.aheadscale.client.tunnel"' "$provider"
ditto -c -k --keepParent "$app" AheadscaleNativeEngine-SIGNED.zip
shasum -a 256 AheadscaleNativeEngine-SIGNED.zip > SIGNED_SHA256SUMS
# При тайм-ауте архив и id сохраняются; повторной отправки не будет.
xcrun notarytool submit AheadscaleNativeEngine-SIGNED.zip --keychain-profile Aheadscale --wait --timeout 10m --output-format json > notarization.json
plutil -extract id raw -o - notarization.json
status=$(plutil -extract status raw -o - notarization.json)
printf '\nstatus: %s\n' "$status"
[ "$status" = Accepted ] || { echo 'Остановлено: заявка пока не Accepted.' >&2; exit 1; }
bash staple-native-engine-on-macbook.sh
