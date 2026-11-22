#!/bin/bash
# Fixed paths on the booted volume. This file is bundled in both Installer phases.
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/sbin:/sbin
umask 077

app=/Applications/Aheadscale.app
principal="$app/Contents/MacOS/AheadscaleExtensionSetup"
engine=/Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine
bridge="$app/Contents/Library/LaunchServices/AheadscaleNativeEngineBridge"
agent="$app/Contents/MacOS/aheadscale"
gui="$app/Contents/Library/LoginItems/Aheadscale UI.app/Contents/MacOS/aheadscale-ui"
cli=/usr/local/bin/aheadscale
state=/var/lib/aheadscale
logdir=/var/log/aheadscale
legacy=/Library/LaunchDaemons/aheadscale.plist
label=Y24464MUL3.ru.aheadscale.client.bridge
native="/Library/LaunchDaemons/$label.plist"
backup=/var/db/aheadscale-native-install
socket=/var/run/aheadscale-native-prototype.sock
team='anchor apple generic and certificate leaf[subject.OU] = "Y24464MUL3"'

die() { echo "Aheadscale: $*" >&2; exit 1; }
present() { [[ -e "$1" || -L "$1" ]]; }
secure() {
    local info owner mode
    [[ ! -L "$1" ]] || return 1
    info=$(stat -f '%u %Lp' "$1") || return 1
    read -r owner mode <<< "$info"
    [[ "$owner" = 0 && "$mode" =~ ^[0-7]+$ ]] && (( (8#$mode & 0022) == 0 ))
}
verify() { secure "$1" && codesign --verify --strict "-R=$team and identifier \"$2\"" "$1"; }
loaded() { launchctl print "system/$1" >/dev/null 2>&1; }
stop_service() {
    if loaded "$1"; then launchctl bootout "system/$1" || return; fi
    local n
    for ((n=0; n<40; n++)); do
        if ! loaded "$1"; then return 0; fi
        sleep .25
    done
    return 1
}
assert_no_gui() {
    local pid name kind
    while read -r pid name; do
        case "$name" in
            aheadscale-ui|AheadscaleExtensionSetup)
                kind=$(osascript -l JavaScript "$scripts/macos-gui-process.js" "$pid" "$1") || return 1
                [[ "$kind" != match ]] || die 'Закройте Aheadscale перед установкой.'
                if [[ "$kind" == unavailable ]] && kill -0 "$pid" 2>/dev/null; then
                    die 'Не удалось проверить работающий процесс Aheadscale.'
                fi
                ;;
        esac
    done <<< "$(ps -axo pid=,ucomm=)"
}
plist_value() { /usr/libexec/PlistBuddy -c "Print :$2" "$1"; }
live() {
    [[ -S "$socket" ]] &&
        perl -e 'alarm shift; exec @ARGV or exit 1' 2 "$agent" status --check live >/dev/null 2>&1
}
