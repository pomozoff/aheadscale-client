"""Сборка payload и hooks из уже проверенных компонентов; без установки на builder."""
import hashlib
import json
from pathlib import Path
import shutil

PAYLOAD = {
    'nm-aheadscale-service': '/usr/libexec/nm-aheadscale-service',
    'nm-aheadscale-auth-dialog': '/usr/libexec/nm-aheadscale-auth-dialog',
    'aheadscale-nm-controller': '/usr/libexec/aheadscale-nm-controller',
    'libnm-vpn-plugin-aheadscale.so': '/usr/lib/NetworkManager/libnm-vpn-plugin-aheadscale.so',
    'libnm-vpn-plugin-aheadscale-editor.so': '/usr/lib/NetworkManager/libnm-vpn-plugin-aheadscale-editor.so',
    'libnm-gtk4-vpn-plugin-aheadscale-editor.so': '/usr/lib/NetworkManager/libnm-gtk4-vpn-plugin-aheadscale-editor.so',
    'nm-aheadscale-service.name': '/usr/lib/NetworkManager/VPN/nm-aheadscale-service.name',
    'nm-aheadscale-service.conf': '/usr/share/dbus-1/system.d/nm-aheadscale-service.conf',
    'aheadscale-nm-controller.service': '/usr/lib/systemd/system/aheadscale-nm-controller.service',
    '20-networkmanager.conf': '/usr/lib/systemd/system/aheadscale.service.d/20-networkmanager.conf',
}


def copy_file(source, target, mode):
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)
    target.chmod(mode)


def stage_networkmanager(root, binaries, staging, cli_only, package_format, channel):
    mode = 'cli' if cli_only else 'desktop'
    origin = {'public': 'https://aheadscale.ru:443', 'internal': 'https://macmini.tailb71cbe.ts.net:10443'}[channel]
    manifest = {'schema_version': 1, 'service_type': 'org.freedesktop.NetworkManager.aheadscale', 'management_url': origin, 'admin_url': origin, 'files': {}, 'variant': mode}
    if not cli_only:
        payload = dict(PAYLOAD)
        if package_format == 'rpm': payload['nm_aheadscale.pp'] = '/usr/share/selinux/packages/nm_aheadscale.pp'
        for source_name, destination in payload.items():
            source = binaries / 'networkmanager' / source_name
            target = staging / destination.lstrip('/')
            copy_file(source, target, 0o755 if destination.startswith('/usr/libexec/') else 0o644)
            manifest['files'][destination] = hashlib.sha256(target.read_bytes()).hexdigest()
        for name in ('LICENSE', 'NOTICES.md', 'SOURCE.json'):
            copy_file(binaries / 'networkmanager' / name, staging / 'usr/share/doc/aheadscale/networkmanager' / name, 0o644)
    helper = root / 'packaging/aheadscale/scripts/networkmanager-lifecycle'
    copy_file(helper, staging / 'usr/lib/aheadscale/networkmanager-lifecycle', 0o755)
    manifest_file = staging / 'usr/share/aheadscale/networkmanager-files.json'
    manifest_file.parent.mkdir(parents=True, exist_ok=True)
    manifest_file.write_text(json.dumps(manifest, sort_keys=True) + '\n')
    normalize_directories(staging)
    return manifest


def normalize_directories(staging):
    for directory in [staging, *staging.rglob('*')]:
        if directory.is_symlink(): raise ValueError('Символическая ссылка в payload пакета')
        if directory.is_dir(): directory.chmod(0o755)


def prepare_hook(root, manifest):
    # Embed the new package's source and manifest: its payload is not unpacked
    # when preinst/%pre runs, and an older helper must not define new behaviour.
    source = (root / 'packaging/aheadscale/scripts/networkmanager-lifecycle').read_text()
    source = source[:source.index("if __name__ == '__main__':")]
    return "python3 - <<'AHEADSCALE_NM_PREPARE'\n" + source + '\nBUILD_MANIFEST = ' + repr(manifest) + "\nimport sys\nsys.argv = ['networkmanager-lifecycle', 'prepare', " + repr(manifest['variant']) + "]\nmain()\nAHEADSCALE_NM_PREPARE\n"


def deb_hooks(root, staging, manifest):
    control = staging / 'DEBIAN'
    control.mkdir(exist_ok=True)
    (control / 'preinst').write_text('#!/bin/sh\nset -eu\ncase "${1:-}" in install|upgrade)\n' + prepare_hook(root, manifest) + ';;\nesac\n')
    (control / 'postinst').write_text('#!/bin/sh\nset -eu\nif [ "${1:-}" = configure ]; then\n/usr/lib/aheadscale/networkmanager-lifecycle apply ' + manifest['variant'] + ' deb\nfi\n')
    (control / 'prerm').write_text('#!/bin/sh\nset -eu\ncase "${1:-}" in remove|deconfigure)\n/usr/lib/aheadscale/networkmanager-lifecycle remove\n/usr/bin/aheadscale service stop\n/usr/lib/aheadscale/cleanup-networkmanager\n/usr/bin/aheadscale service uninstall\n;;\nesac\n')
    for name in ('preinst', 'postinst', 'prerm'): (control / name).chmod(0o755)
