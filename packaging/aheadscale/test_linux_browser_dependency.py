"""Установщик GUI доставляет xdg-open; серверный CLI остаётся независимым."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


class BrowserDependencyTest(unittest.TestCase):
    def test_deb_desktop_requires_browser_launcher_but_cli_does_not(self):
        for cli in (False, True):
            with self.subTest(cli=cli), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                source = Path(__file__).resolve().parent
                package_dir = root / 'packaging/aheadscale'
                shutil.copytree(source, package_dir, ignore=shutil.ignore_patterns('__pycache__'))
                (root / 'LICENSE').write_text('license fixture\n')
                binaries = root / 'build/linux-amd64-public'
                component = binaries / 'networkmanager'
                component.mkdir(parents=True)
                for name in ('aheadscale', 'aheadscale-ui'):
                    (binaries / name).write_bytes(b'binary fixture')
                import networkmanager_package
                for name in (*networkmanager_package.PAYLOAD,
                             networkmanager_package.PLASMA_PLUGIN,
                             'LICENSE', 'NOTICES.md', 'SOURCE.json'):
                    (component / name).write_bytes(b'component fixture')
                command = [sys.executable, str(package_dir / 'package.py'),
                           '--platform', 'linux', '--arch', 'amd64', '--channel', 'public']
                if cli:
                    command.append('--cli-only')
                result = subprocess.run(command, capture_output=True, text=True,
                                        env=dict(os.environ, PYTHONDONTWRITEBYTECODE='1'), timeout=30)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                package = next((root / 'dist').glob('*.deb'))
                depends = subprocess.check_output(['dpkg-deb', '-f', str(package), 'Depends'], text=True)
                dependencies = {item.strip().split()[0] for item in depends.split(',')}
                if cli:
                    self.assertNotIn('xdg-utils', dependencies)
                else:
                    self.assertIn('xdg-utils', dependencies,
                                  'GUI invokes xdg-open: apt must install its provider')


if __name__ == '__main__':
    unittest.main()
