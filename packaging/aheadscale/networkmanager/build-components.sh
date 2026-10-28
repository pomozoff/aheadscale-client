#!/bin/sh
# Компоненты одного native Linux target; исходники проверяются до исполнения.
set -eu
output=${1:?Нужен каталог готовых бинарников клиента}
project=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
output=$(CDPATH= cd -- "$output" && pwd)
case "$output" in "$project"/build/linux-*) ;; *) echo 'Неверный каталог Linux target' >&2; exit 1;; esac
[ "$(go env GOVERSION)" = go1.26.7 ] || { echo 'Нужен Go 1.26.7' >&2; exit 1; }
[ "$(go env GOARCH)" = "$(go env GOHOSTARCH)" ] || { echo 'GTK-компоненты собираются на нативном runner выбранной архитектуры' >&2; exit 1; }
[ "$(meson --version)" = 1.3.2 ] || { echo 'Нужен Meson 1.3.2 из закреплённого Ubuntu 24.04 builder' >&2; exit 1; }
workspace=$(mktemp -d "${TMPDIR:-/tmp}/aheadscale-nm-source.XXXXXX")
trap 'rm -rf -- "$workspace"' EXIT HUP INT TERM
python3 "$project/packaging/aheadscale/networkmanager/prepare-source.py" "$project" "$workspace"
cd "$workspace/source"
# Load the original and adapted libnm factories together in the C regression.
cc -shared -fPIC $(pkg-config --cflags libnm) properties/nm-netbird-editor-plugin.c -o "$workspace/upstream-plugin.so" $(pkg-config --libs libnm) -ldl
git apply "$project/packaging/aheadscale/networkmanager/aheadscale.patch"
mkdir -p "$output/networkmanager"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -p 2 -ldflags '-s -w' -o "$output/networkmanager/nm-aheadscale-service" ./cmd/nm-aheadscale-service
CGO_ENABLED=0 go build -buildvcs=false -trimpath -p 2 -ldflags '-s -w' -o "$output/networkmanager/nm-aheadscale-auth-dialog" ./cmd/nm-aheadscale-auth-dialog
meson setup "$workspace/editor" -Dgtk4=true -Dupstream_plugin="$workspace/upstream-plugin.so"
meson compile -C "$workspace/editor" -j 2
meson test -C "$workspace/editor" --print-errorlogs
for library in libnm-vpn-plugin-aheadscale.so libnm-vpn-plugin-aheadscale-editor.so libnm-gtk4-vpn-plugin-aheadscale-editor.so; do
    cp "$workspace/editor/properties/$library" "$output/networkmanager/$library"
done
cp packaging/NetworkManager/VPN/nm-aheadscale-service.name "$output/networkmanager/"
cp packaging/dbus-1/system.d/nm-aheadscale-service.conf "$output/networkmanager/"
cp LICENSE "$output/networkmanager/LICENSE"
printf '%s\n' '# Сторонние компоненты NetworkManager' '' 'Основано на netbirdio/network-manager-vpn-plugin v0.1.12 (BSD-3-Clause).' 'Изменения Aheadscale и закреплённые версии перечислены в SOURCE.json.' 'Технические Go module/imports сохранены для совместимости. Основной клиент включает NetBird; см. общие LICENSE и NOTICES.md.' > "$output/networkmanager/NOTICES.md"
cp "$project/packaging/aheadscale/networkmanager/aheadscale-nm-controller.service" "$output/networkmanager/"
cp "$project/packaging/aheadscale/networkmanager/20-networkmanager.conf" "$output/networkmanager/"
cp "$output/aheadscale-nm-controller" "$output/networkmanager/"
if [ -n "${AHEADSCALE_SELINUX_POLICY:-}" ]; then
    cp "$AHEADSCALE_SELINUX_POLICY" "$output/networkmanager/nm_aheadscale.pp"
fi
python3 - "$project" "$output" <<'PY'
import hashlib,json,os,subprocess,sys
from pathlib import Path
root,out=map(Path,sys.argv[1:])
value=json.loads((root/'packaging/aheadscale/networkmanager-source.json').read_text())
value['client_revision']=os.environ.get('AHEADSCALE_SOURCE_REVISION','')
value['architecture']=subprocess.check_output(['go','env','GOARCH'],text=True).strip()
value['component_sha256']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((out/'networkmanager').iterdir()) if p.is_file()}
(out/'networkmanager/SOURCE.json').write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n')
PY
