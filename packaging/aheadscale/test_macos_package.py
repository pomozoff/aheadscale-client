"""Регрессии метаданных установщика macOS без запуска системного Installer."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


class MacOSPackageTest(unittest.TestCase):
    def test_installer_overrides_old_relocatable_default(self):
        self.package_fixture('old-true')

    def test_installer_adds_explicit_policy_when_builder_omits_default(self):
        self.package_fixture('new-implicit-false')

    def package_fixture(self, analyzer_default):
        source = Path(__file__).resolve().parent
        with tempfile.TemporaryDirectory(prefix='aheadscale-pkg-policy-') as directory:
            root = Path(directory)
            package_dir = root / 'packaging/aheadscale'
            package_dir.mkdir(parents=True)
            shutil.copy2(source / 'package.py', package_dir / 'package.py')
            (package_dir / 'version.txt').write_text('0.80.0-aheadscale.25\n')
            (root / 'LICENSE').write_text('license fixture\n')
            (package_dir / 'NOTICES.md').write_text('notices fixture\n')
            (package_dir / 'icon.icns').write_bytes(b'icon fixture')
            scripts = package_dir / 'scripts'
            scripts.mkdir()
            for name in ['macos-preinstall', 'macos-postinstall',
                         'macos-uninstall.command', 'macos-gui-process.js']:
                shutil.copy2(source / 'scripts' / name, scripts / name)
            binaries = root / 'build/darwin-arm64-public'
            binaries.mkdir(parents=True)
            for name in ['aheadscale', 'aheadscale-ui']:
                (binaries / name).write_bytes(b'unmodified-binary-fixture')
            tools = root / 'tools'
            tools.mkdir()
            # Mock platform-only tools; run the real Python packager. The
            # analyzer deliberately emits the old macOS default (relocatable).
            tool = tools / 'platform-tool'
            tool.write_text('#!' + sys.executable + '\n' + r'''
import json, os, pathlib, plistlib, sys
name=pathlib.Path(sys.argv[0]).name
args=sys.argv[1:]
if name != 'pkgbuild':
    sys.exit(0)
if '--analyze' in args:
    component={'RootRelativeBundlePath':'Applications/Aheadscale.app',
               'BundleHasStrictIdentifier':True,'BundleOverwriteAction':'upgrade'}
    if os.environ['ANALYZER_DEFAULT']=='old-true':component['BundleIsRelocatable']=True
    plistlib.dump([component],open(args[-1],'wb'))
    sys.exit(0)
if '--component-plist' not in args:
    sys.exit('bundle relocation must be explicitly disabled')
components=plistlib.load(open(args[args.index('--component-plist')+1],'rb'))
app=next(c for c in components if c['RootRelativeBundlePath']=='Applications/Aheadscale.app')
if app.get('BundleIsRelocatable') is not False:
    sys.exit('analyzer relocation default was not overridden')
if app['BundleHasStrictIdentifier'] is not True or app['BundleOverwriteAction']!='upgrade':
    sys.exit('unrelated installer policy changed')
pathlib.Path(os.environ['AUDIT']).write_text(json.dumps(app))
pathlib.Path(args[-1]).write_bytes(b'package-fixture')
''')
            tool.chmod(0o755)
            for name in ['codesign', 'pkgbuild', 'pkgutil']:
                (tools / name).symlink_to(tool)
            audit = root / 'audit.json'
            env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'],
                       AUDIT=str(audit), ANALYZER_DEFAULT=analyzer_default, PYTHONDONTWRITEBYTECODE='1')
            result = subprocess.run(
                [sys.executable, str(package_dir / 'package.py'), '--platform',
                 'darwin', '--arch', 'arm64', '--channel', 'public'],
                env=env, capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            policy = json.loads(audit.read_text())
            self.assertIs(policy['BundleIsRelocatable'], False)
            for name in ['aheadscale', 'aheadscale-ui']:
                self.assertEqual((binaries / 'desktop-package/Applications/Aheadscale.app/Contents/MacOS' / name).read_bytes(),
                                 b'unmodified-binary-fixture')


if __name__ == '__main__':
    unittest.main()
