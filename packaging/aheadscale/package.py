#!/usr/bin/env python3
"""Package already-built Aheadscale binaries; never install them on the builder."""
import argparse
import os
from pathlib import Path
import plistlib
import shutil
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--platform', choices=['linux', 'darwin', 'windows'], required=True)
parser.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
parser.add_argument('--channel', choices=['public', 'internal'], required=True)
parser.add_argument('--version', default='0.80.0-aheadscale.11')
parser.add_argument('--cli-only', action='store_true')
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
binaries = root / 'build' / f'{args.platform}-{args.arch}-{args.channel}'
staging = binaries / ('cli-package' if args.cli_only else 'desktop-package')
# Staging is only our build output; refuse ambiguous or reused paths.
if staging.exists():
    raise SystemExit('Package staging already exists; choose a fresh build output')
staging.mkdir(parents=True)
output = root / 'dist'
output.mkdir(exist_ok=True)
name = f'aheadscale_{args.version}_{args.platform}_{args.arch}_{args.channel}'
if args.cli_only:
    name += '_cli'
scripts = root / 'packaging/aheadscale/scripts'


def copy(source, target):
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, target)


if args.platform == 'linux':
    copy(binaries / 'aheadscale', staging / 'usr/bin/aheadscale')
    cleanup = staging / 'usr/lib/aheadscale/cleanup-networkmanager'
    copy(scripts / 'cleanup-networkmanager', cleanup)
    cleanup.chmod(0o755)
    if not args.cli_only:
        copy(binaries / 'aheadscale-ui', staging / 'usr/bin/aheadscale-ui')
        copy(root / 'packaging/aheadscale/icon.png', staging / 'usr/share/icons/hicolor/512x512/apps/aheadscale.png')
        desktop = staging / 'usr/share/applications/org.wails.aheadscale.desktop'
        desktop.parent.mkdir(parents=True, exist_ok=True)
        desktop.write_text(
            '[Desktop Entry]\nType=Application\nName=Aheadscale\n'
            'Exec=/usr/bin/aheadscale-ui\nIcon=aheadscale\nTerminal=false\n'
            'Categories=Network;\nStartupWMClass=org.wails.aheadscale\n')
    copy(root / 'LICENSE', staging / 'usr/share/doc/aheadscale/LICENSE')
    copy(root / 'packaging/aheadscale/NOTICES.md', staging / 'usr/share/doc/aheadscale/NOTICES.md')
    control = staging / 'DEBIAN'
    control.mkdir()
    depends = 'iproute2, iptables'
    if not args.cli_only:
        depends += ', libgtk-4-1 (>= 4.14), libwebkitgtk-6.0-4'
    control.joinpath('control').write_text(
        f'Package: aheadscale\nVersion: {args.version.replace("-", "~", 1)}\n'
        f'Architecture: {args.arch}\nMaintainer: Aheadscale <hello@aheadscale.ru>\n'
        f'Depends: {depends}\nConflicts: aheadscale-cli\n'
        'Section: net\nPriority: optional\nDescription: Aheadscale private network client\n')
    for source, target in [('postinst', 'postinst'), ('prerm', 'prerm')]:
        copy(scripts / source, control / target)
        control.joinpath(target).chmod(0o755)
    # Installer is one package containing both UI and service; no second download.
    subprocess.run(['dpkg-deb', '--root-owner-group', '--build', str(staging), str(output / (name + '.deb'))], check=True)
    subprocess.run(['dpkg-deb', '--info', str(output / (name + '.deb'))], check=True)
elif args.platform == 'darwin':
    if args.cli_only:
        raise SystemExit('macOS uses a single desktop installer')
    app = staging / 'Applications/Aheadscale.app/Contents'
    for binary in ['aheadscale', 'aheadscale-ui']:
        copy(binaries / binary, app / 'MacOS' / binary)
    copy(root / 'packaging/aheadscale/icon.icns', app / 'Resources/icons.icns')
    copy(root / 'LICENSE', app / 'Resources/LICENSE')
    copy(root / 'packaging/aheadscale/NOTICES.md', app / 'Resources/NOTICES.md')
    copy(scripts / 'macos-uninstall.command', staging / 'Applications/Uninstall Aheadscale.command')
    staging.joinpath('Applications/Uninstall Aheadscale.command').chmod(0o755)
    info = {
        'CFBundlePackageType': 'APPL', 'CFBundleName': 'Aheadscale',
        'CFBundleDisplayName': 'Aheadscale', 'CFBundleIdentifier': 'ru.aheadscale.client',
        'CFBundleExecutable': 'aheadscale-ui', 'CFBundleVersion': '0.80.0',
        'CFBundleShortVersionString': '0.80.0', 'CFBundleIconFile': 'icons',
        'LSMinimumSystemVersion': '11.0', 'NSHighResolutionCapable': True,
        'LSUIElement': True, 'NSHumanReadableCopyright': 'Aheadscale; includes NetBird, see LICENSE',
    }
    app.joinpath('Info.plist').write_bytes(plistlib.dumps(info))
    # Ad-hoc signing is only for executing local test artifacts, not Developer ID.
    subprocess.run(['codesign', '--force', '--deep', '--sign', '-', str(app.parent)], check=True)
    package_scripts = binaries / 'pkg-scripts'
    package_scripts.mkdir()
    for source, target in [('macos-preinstall', 'preinstall'), ('macos-postinstall', 'postinstall')]:
        copy(scripts / source, package_scripts / target)
        package_scripts.joinpath(target).chmod(0o755)
    subprocess.run(['pkgbuild', '--root', str(staging), '--identifier', 'ru.aheadscale.client',
                    '--version', '0.80.0', '--scripts', str(package_scripts), '--install-location', '/',
                    str(output / (name + '.pkg'))], check=True)
    subprocess.run(['pkgutil', '--payload-files', str(output / (name + '.pkg'))], check=True)
else:
    # Reuse upstream NSIS control flow and privilege-preserving UI launcher.
    original = (root / 'client/installer.nsis').read_text()
    original = original.replace('Netbird', 'Aheadscale').replace('NetBird', 'Aheadscale').replace('netbird', 'aheadscale')
    original = original.replace('Aheadscale.io', 'aheadscale.ru')
    original = original.replace('Aheadscale Authors, 2022', 'NetBird Authors and Aheadscale contributors')
    original = original.replace('service install\'', 'service install --disable-profiles\'')
    # Upstream's NSIS expects root/client and dist layouts. Build a self-contained tree.
    work = binaries / 'nsis'
    client = work / 'client'
    client.mkdir(parents=True)
    copy(root / 'LICENSE', work / 'LICENSE')
    copy(root / 'client/ui/build/windows/nsis/wails_tools.nsh', client / 'ui/build/windows/nsis/wails_tools.nsh')
    copy(root / 'packaging/aheadscale/icon.ico', client / 'ui/build/windows/icon.ico')
    copy(root / 'packaging/aheadscale/banner.bmp', client / 'ui/build/banner.bmp')
    copy(root / 'packaging/aheadscale/icon.png', client / 'ui/assets/aheadscale.png')
    payload = work / f'dist/aheadscale_windows_{args.arch}'
    for binary in ['aheadscale.exe', 'aheadscale-ui.exe', 'wintun.dll']:
        copy(binaries / binary, payload / binary)
    copy(binaries / 'WINTUN-LICENSE.txt', payload / 'WINTUN-LICENSE.txt')
    copy(binaries / 'MicrosoftEdgeWebview2Setup.exe', client / 'MicrosoftEdgeWebview2Setup.exe')
    copy(root / 'LICENSE', payload / 'LICENSE')
    copy(root / 'packaging/aheadscale/NOTICES.md', payload / 'NOTICES.md')
    installer = client / 'installer.nsis'
    installer.write_text(original)
    makensis = os.environ.get('MAKENSIS', 'makensis')
    env = dict(os.environ, APPVER='0.80.0.11')
    subprocess.run([makensis, '-V2', '-DARCH=' + args.arch, str(installer)], cwd=work, env=env, check=True)
    shutil.copy2(work / 'aheadscale-installer.exe', output / (name + '.exe'))
print('Created preview package: ' + name)
