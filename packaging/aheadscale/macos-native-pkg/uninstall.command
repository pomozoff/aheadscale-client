#!/bin/bash
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/sbin:/sbin
app=/Applications/Aheadscale.app
main="$app/Contents/MacOS/AheadscaleExtensionSetup"
agent="$app/Contents/MacOS/aheadscale"
gui="$app/Contents/Library/LoginItems/Aheadscale UI.app/Contents/MacOS/aheadscale-ui"
label=Y24464MUL3.ru.aheadscale.client.bridge
cli=/usr/local/bin/aheadscale
verify_app() {
    [[ -d "$app" && ! -L "$app" && -x "$main" && ! -L "$main" ]] &&
        codesign --verify --deep --strict '-R=anchor apple generic and certificate leaf[subject.OU] = "Y24464MUL3" and identifier "ru.aheadscale.client"' "$app"
}
assert_gui_closed() {
    local pid result
    for pid in $(pgrep -x aheadscale-ui || true); do
        result=$(osascript -l JavaScript "$app/Contents/Resources/macos-gui-process.js" "$pid" "$gui")
        if [[ "$result" = match || "$result" = unavailable ]] && kill -0 "$pid" 2>/dev/null; then
            echo 'Выйдите из Aheadscale через меню приложения и повторите удаление.' >&2
            exit 1
        fi
    done
}
verify_app
assert_gui_closed
if [[ $EUID != 0 ]]; then
    [[ $# = 0 ]] || exit 1
    sudo -v
    "$main" --remove-system-vpn
    exec sudo '/Applications/Uninstall Aheadscale.command' --remove-files
fi
[[ $# = 1 && "$1" = --remove-files ]] || { echo 'Запустите удаление из сеанса macOS.' >&2; exit 1; }
[[ ! -e /var/db/aheadscale-native-install ]] || { echo 'Обнаружена незавершённая установка.' >&2; exit 1; }
if launchctl print "system/$label" >/dev/null 2>&1; then launchctl bootout "system/$label"; fi
for ((attempt=0; attempt<40; attempt++)); do
    if ! launchctl print "system/$label" >/dev/null 2>&1; then break; fi
    sleep .25
done
if launchctl print "system/$label" >/dev/null 2>&1; then
    echo 'Служба Aheadscale ещё не остановилась.' >&2
    exit 1
fi
rm -f "/Library/LaunchDaemons/$label.plist" /Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine
if [[ -L "$cli" && "$(readlink "$cli")" = "$agent" ]]; then rm -f "$cli"; fi
rm -rf "$app"
rm -f '/Applications/Uninstall Aheadscale.command'
pkgutil --forget ru.aheadscale.client.native.pkg >/dev/null
echo 'Aheadscale удалён. Данные регистрации сохранены.'
