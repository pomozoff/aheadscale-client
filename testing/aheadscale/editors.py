#!/usr/bin/env python3
"""Настоящие C/GTK-регрессии из закреплённого исходника NM-плагина."""
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def main():
    if hasattr(os, 'geteuid') and os.geteuid() == 0:
        raise SystemExit('Редакторы проверяются обычным CI-пользователем')
    meson_version = subprocess.check_output(['meson', '--version'], text=True).strip()
    if meson_version != '1.3.2':
        raise SystemExit('Нужен Meson 1.3.2 из Ubuntu 24.04 builder')
    with tempfile.TemporaryDirectory(prefix='aheadscale-ci-editors-') as directory:
        work = Path(directory)
        subprocess.run([sys.executable, str(ROOT / 'packaging/aheadscale/networkmanager/prepare-source.py'),
                        str(ROOT), str(work)], check=True)
        source = work / 'source'
        cflags = shlex.split(subprocess.check_output(['pkg-config', '--cflags', 'libnm'], text=True))
        libs = shlex.split(subprocess.check_output(['pkg-config', '--libs', 'libnm'], text=True))
        # Исходная factory компилируется до патча: регрессия загружает обе настоящие библиотеки.
        upstream = work / 'upstream-plugin.so'
        subprocess.run(['cc', '-shared', '-fPIC', *cflags,
                        'properties/nm-netbird-editor-plugin.c', '-o', str(upstream), *libs, '-ldl'],
                       cwd=source, check=True)
        subprocess.run(['git', 'apply', '--unidiff-zero',
                        str(ROOT / 'packaging/aheadscale/networkmanager/aheadscale.patch')],
                       cwd=source, check=True)
        build = work / 'editor'
        subprocess.run(['meson', 'setup', str(build), str(source), '-Dgtk4=true',
                        '-Dupstream_plugin=' + str(upstream)], check=True)
        subprocess.run(['meson', 'compile', '-C', str(build), '-j', '2'], check=True)
        subprocess.run(['meson', 'test', '-C', str(build), '--no-rebuild', '--print-errorlogs'], check=True)
    print('C/GTK редакторы: модель и обе factory проверены; временный исходник и сборка удалены')


if __name__ == '__main__':
    main()
