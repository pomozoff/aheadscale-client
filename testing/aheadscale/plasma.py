#!/usr/bin/env python3
"""Нативный KDE producer и проверка происхождения его артефакта."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
PIN = ROOT / 'packaging/aheadscale/networkmanager-source.json'
ARCHES = {'x86_64': 'amd64', 'aarch64': 'arm64'}
PROVENANCE_KEYS = ('upstream_commit', 'adapted_commit', 'archive_sha256',
                   'patch_sha256', 'plasma_sdk', 'plasma_toolchain')
COMPONENT = 'plasmanetworkmanagement_aheadscaleui.so'


def write_verified(path, body, expected):
    if hashlib.sha256(body).hexdigest() != expected:
        raise ValueError('SHA256 SDK не совпадает: ' + path.name)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('xb') as f:
        f.write(body)


def validate_environment(pin, observed):
    if observed.get('architecture') not in {'amd64', 'arm64'}:
        raise ValueError('Неизвестная нативная архитектура KDE')
    for key, value in pin['plasma_toolchain'].items():
        if observed.get(key) != value:
            raise ValueError(f'Неверный KDE toolchain {key}: {observed.get(key)!r}, нужен {value!r}')


def check_component(path, arch):
    if arch not in {'amd64', 'arm64'} or path.is_symlink():
        raise ValueError('Неизвестная архитектура или symlink компонента')
    with path.open('rb') as f:
        data = f.read(64)
    if (len(data) < 64 or data[:7] != b'\x7fELF\x02\x01\x01'
            or struct.unpack_from('<HH', data, 16) != (3, {'amd64': 62, 'arm64': 183}[arch])):
        raise ValueError('Нужен нативный 64-bit ELF shared object')


def verify_provenance(pin, meta, component, arch):
    expected = pin.get('plasma_component_sha256', {}).get(arch)
    if not expected:
        raise ValueError('Нет доверенного SHA256 компонента для ' + arch)
    check_component(component, arch)
    with component.open('rb') as f:
        actual = hashlib.file_digest(f, 'sha256').hexdigest()
    if actual != expected or meta.get('component_sha256') != actual:
        raise ValueError('SHA256 KDE-компонента не совпадает')
    if meta.get('architecture') != arch or not re.fullmatch('[0-9a-f]{40}', meta.get('client_revision', '')):
        raise ValueError('Некорректная архитектура/ревизия producer')
    for key in PROVENANCE_KEYS:
        if meta.get(key) != pin[key]:
            raise ValueError('Другое происхождение KDE-компонента: ' + key)


def output(argv):
    return subprocess.check_output(argv, text=True).strip()


def package_version(package):
    full = output(['dpkg-query', '-W', '-f=${Version}', package])
    match = re.match(r'^(?:[0-9]+:)?([0-9]+\.[0-9]+\.[0-9]+)', full)
    if not match:
        raise ValueError('Неизвестная версия пакета ' + package)
    return match.group(1)


def environment():
    release = platform.freedesktop_os_release()
    return {'os': release['NAME'] + ' ' + release['VERSION_ID'],
            'architecture': ARCHES.get(platform.machine(), platform.machine()),
            'qt': output(['pkg-config', '--modversion', 'Qt6Widgets']),
            'kf': package_version('libkf6coreaddons-dev'),
            'plasma': package_version('plasma-nm'),
            'cmake': output(['/usr/bin/cmake', '--version']).splitlines()[0].split()[-1],
            'gcc': output(['g++', '-dumpfullversion'])}


def build(pin, destination):
    if hasattr(os, 'geteuid') and os.geteuid() == 0:
        raise ValueError('Producer выполняется обычным пользователем hosted runner')
    observed = environment()
    print(json.dumps(observed, ensure_ascii=False), flush=True)
    validate_environment(pin, observed)
    revision = os.environ.get('GITHUB_SHA') or output(['git', '-C', str(ROOT), 'rev-parse', 'HEAD'])
    if not re.fullmatch('[0-9a-f]{40}', revision):
        raise ValueError('Нужна точная ревизия producer')
    destination.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='aheadscale-ci-plasma-') as directory:
        work = Path(directory)
        subprocess.run([sys.executable, str(ROOT/'packaging/aheadscale/networkmanager/prepare-source.py'),
                        str(ROOT), str(work)], check=True)
        source = work/'source'
        subprocess.run(['git', 'apply', '--unidiff-zero', str(ROOT/'packaging/aheadscale/networkmanager/aheadscale.patch')], cwd=source, check=True)
        sdk = work/'sdk'
        commit = pin['plasma_sdk']['commit']
        if not re.fullmatch('[0-9a-f]{40}', commit):
            raise ValueError('Нужен точный SDK commit')
        headers = pin['plasma_sdk']['header_sha256']
        if set(headers) != {'libs/editor/vpnuiplugin.h', 'libs/editor/widgets/settingwidget.h'}:
            raise ValueError('Неожиданный набор SDK-заголовков')
        for name, digest in headers.items():
            url = 'https://invent.kde.org/plasma/plasma-nm/-/raw/' + commit + '/' + name
            with urllib.request.urlopen(url, timeout=60) as response:
                body = response.read(1024 * 1024 + 1)
            write_verified(sdk/name, body, digest)
        compiled = work/'build'
        subprocess.run(['/usr/bin/cmake', '-S', str(source/'plasma'), '-B', str(compiled),
                        '-DCMAKE_BUILD_TYPE=Release', '-DPLASMA_NM_SOURCE=' + str(sdk)], check=True)
        subprocess.run(['/usr/bin/cmake', '--build', str(compiled), '-j2'], check=True)
        report = work/'ctest.xml'
        try:
            subprocess.run(['/usr/bin/ctest', '--test-dir', str(compiled), '--output-on-failure',
                            '--no-tests=error', '--output-junit', str(report)], check=True)
        finally:
            if report.exists():
                shutil.copyfile(report, destination/'ctest.xml')
        component = compiled/'plugins/plasma/network/vpn'/COMPONENT
        check_component(component, observed['architecture'])
        copied = destination/COMPONENT
        shutil.copyfile(component, copied)
        with copied.open('rb') as f:
            digest = hashlib.file_digest(f, 'sha256').hexdigest()
        meta = {key: pin[key] for key in PROVENANCE_KEYS}
        meta.update(schema=1, client_revision=revision, architecture=observed['architecture'],
                    environment=observed, component_sha256=digest,
                    pinMatch=pin.get('plasma_component_sha256', {}).get(observed['architecture']) == digest)
        (destination/'provenance.json').write_text(json.dumps(meta, ensure_ascii=False, indent=2) + '\n')
        print('KDE factory/SSO/Cancel: PASS; architecture=' + observed['architecture'] + '; sha256=' + digest, flush=True)
        print('Кандидат компонента; включение в установщик требует доверенного SHA256 в pin', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--verify-component', type=Path)
    parser.add_argument('--provenance', type=Path)
    parser.add_argument('--arch', choices=['amd64', 'arm64'])
    args = parser.parse_args()
    pin = json.loads(PIN.read_text())
    if args.verify_component:
        if not args.provenance or not args.arch:
            parser.error('--provenance и --arch обязательны при проверке')
        verify_provenance(pin, json.loads(args.provenance.read_text()), args.verify_component, args.arch)
    elif args.output:
        build(pin, args.output.resolve())
    else:
        parser.error('Нужен --output или --verify-component')


if __name__ == '__main__':
    main()
