#!/bin/bash
set -euo pipefail
app=/Applications/Aheadscale.app
agent="$app/Contents/MacOS/aheadscale"
if [[ $EUID != 0 ]]; then
    exec sudo /bin/bash "$0"
fi
if [[ ! -x "$agent" ]]; then
    echo 'Aheadscale is not installed at the expected location.' >&2
    exit 1
fi
if [[ -f /Library/LaunchDaemons/aheadscale.plist ]]; then
    "$agent" service stop
    "$agent" service uninstall
fi
pkill -x aheadscale-ui || true
if [[ -L /usr/local/bin/aheadscale && "$(readlink /usr/local/bin/aheadscale)" == "$agent" ]]; then
    rm /usr/local/bin/aheadscale
fi
# Keep account keys and user preferences for a later reinstall.
rm -rf "$app"
rm -f '/Applications/Uninstall Aheadscale.command'
pkgutil --forget ru.aheadscale.client
echo 'Aheadscale removed. Account keys and preferences were retained.'
