"""Изолированные сценарии установки; реальные службы и пользовательские данные не трогаются."""
import hashlib
import importlib.machinery
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

loader = importlib.machinery.SourceFileLoader('lifecycle', str(Path(__file__).parent / 'scripts/networkmanager-lifecycle'))
spec = importlib.util.spec_from_loader(loader.name, loader)
lifecycle = importlib.util.module_from_spec(spec)
loader.exec_module(lifecycle)

class LifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.calls = []
        self.files = {'/usr/libexec/nm-aheadscale-service': hashlib.sha256(b'plugin').hexdigest()}
        self.manifest = {'schema_version': 1, 'service_type': lifecycle.SERVICE, 'management_url': 'https://aheadscale.ru:443', 'admin_url': 'https://aheadscale.ru:443', 'files': self.files}
        self.installer = lifecycle.Installer(self.root, self.manifest, self.calls.append)
        self.put('/proc/sys/kernel/random/boot_id', 'boot')
        self.put('/var/lib/aheadscale/default.json', 'private-key-preserved')
        self.put('/home/alice/.config/autostart/aheadscale.desktop', 'autostart-preserved')
    def put(self, name, text):
        p = self.root / name.lstrip('/')
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text)
        p.chmod(0o644)
        for parent in p.parents:
            if parent == self.root: break
            if parent.is_dir(): parent.chmod(0o755)
        return p
    def payload(self):
        self.put('/usr/libexec/nm-aheadscale-service', 'plugin')
    def test_install_upgrade_remove_reinstall_and_foreign_uuid(self):
        self.installer.preflight('desktop')
        self.payload()
        first = self.installer.install('desktop')
        self.assertTrue(first['uuid'])
        public = json.loads((self.root / lifecycle.PUBLIC.lstrip('/')).read_text())
        self.assertNotIn('files', public)
        self.assertNotIn('private', json.dumps(public))
        self.installer.preflight('desktop')
        self.assertEqual(first['uuid'], self.installer.install('desktop')['uuid'])
        self.installer.remove()
        self.assertEqual(self.calls, [['/usr/libexec/aheadscale-nm-controller', '--remove-profile']])
        self.assertFalse((self.root / lifecycle.PUBLIC.lstrip('/')).exists())
        self.payload()
        self.assertEqual(first['uuid'], self.installer.install('desktop')['uuid'])
        self.assertEqual((self.root / 'var/lib/aheadscale/default.json').read_text(), 'private-key-preserved')
        self.assertEqual((self.root / 'home/alice/.config/autostart/aheadscale.desktop').read_text(), 'autostart-preserved')
    def test_upgrade_replaces_only_previously_verified_payload(self):
        self.payload()
        first = self.installer.install('desktop')
        self.installer.preflight('desktop')
        self.put('/usr/libexec/nm-aheadscale-service', 'new-plugin')
        self.manifest['files'] = {'/usr/libexec/nm-aheadscale-service': hashlib.sha256(b'new-plugin').hexdigest()}
        self.assertEqual(first['uuid'], self.installer.install('desktop')['uuid'])
    def test_foreign_file_and_changed_receipt_are_refused(self):
        self.put('/usr/libexec/nm-aheadscale-service', 'foreign')
        with self.assertRaises(ValueError): self.installer.preflight('desktop')
        self.payload()
        self.installer.install('desktop')
        receipt = self.root / lifecycle.PRIVATE.lstrip('/')
        receipt.chmod(0o666)
        with self.assertRaises(ValueError): self.installer.preflight('desktop')
        receipt.chmod(0o600)
        self.put('/usr/libexec/nm-aheadscale-service', 'edited')
        with self.assertRaises(ValueError): self.installer.remove()
        self.assertEqual(self.calls, [])
    def test_off_on_transfers_do_not_rewrite_preferences(self):
        for on in (False, True):
            self.payload()
            self.installer.install('desktop')
            self.installer.transfer_intent(on)
            state = json.loads((self.root / lifecycle.STATE.lstrip('/')).read_text())
            self.assertEqual(state['Intent']['Connected'], on)
            self.assertEqual(state['BootID'], 'boot')
            self.installer.install('cli')
            marker = json.loads((self.root / lifecycle.HANDOVER.lstrip('/')).read_text())
            self.assertEqual(marker['connected'], on)
            self.assertEqual((self.root / 'var/lib/aheadscale/default.json').read_text(), 'private-key-preserved')
    def test_unknown_unit_and_parent_symlink_are_refused(self):
        self.put('/etc/systemd/system/aheadscale.service', '[Service]\nExecStart=/bin/other\n')
        with self.assertRaises(ValueError): self.installer.preflight('desktop')
        (self.root / 'etc/systemd/system/aheadscale.service').unlink()
        outside = self.root / 'foreign'
        outside.mkdir()
        (self.root / 'var/lib/aheadscale').rename(self.root / 'saved')
        (self.root / 'var/lib/aheadscale').symlink_to(outside)
        with self.assertRaises(ValueError): self.installer.install('desktop')
    def test_prepare_apply_preserves_stopped_and_active_units(self):
        from types import SimpleNamespace
        self.put('/usr/lib/systemd/system/aheadscale.service', '[Unit]\nDescription=Aheadscale private network\n[Service]\nExecStart=/usr/bin/aheadscale service run\n')
        for active, enabled, on in [('active', 'enabled', True), ('inactive', 'disabled', False)]:
            self.payload()
            self.installer.install('desktop')
            calls = []
            def run(args):
                calls.append(args)
                output = active if args[:2] == ['systemctl', 'is-active'] else enabled if args[:2] == ['systemctl', 'is-enabled'] else json.dumps({'daemonStatus': 'Connected' if on else 'Idle'})
                return SimpleNamespace(returncode=0, stdout=output, stderr='')
            self.installer.runner = run
            self.installer.transfer_intent(on)
            self.installer.prepare('desktop')
            self.installer.apply('desktop', 'rpm')
            expected = 'restart' if active == 'active' else 'stop'
            self.assertIn(['systemctl', expected, 'aheadscale.service'], calls)
            state = json.loads((self.root / lifecycle.STATE.lstrip('/')).read_text())
            self.assertEqual(state['Intent']['Connected'], on)
    def test_changed_selinux_module_is_refused_before_cleanup(self):
        from types import SimpleNamespace
        policy = '/usr/share/selinux/packages/nm_aheadscale.pp'
        self.manifest['files'][policy] = hashlib.sha256(b'policy').hexdigest()
        self.payload()
        self.put(policy, 'policy')
        receipt = self.installer.install('desktop')
        receipt['selinux_module_fingerprint'] = '400 nm_aheadscale pp sha256:trusted'
        self.installer.write_json(lifecycle.PRIVATE, receipt, 0o600)
        commands = []
        def foreign(args):
            commands.append(args)
            return SimpleNamespace(returncode=0, stdout='400 nm_aheadscale pp sha256:foreign', stderr='')
        self.installer.runner = foreign
        with self.assertRaises(ValueError): self.installer.remove()
        self.assertEqual(commands, [['semodule', '-lfull', '-m']])
    def test_descriptor_is_not_a_key_store(self):
        self.payload()
        self.installer.install('desktop')
        pub = self.root / lifecycle.PUBLIC.lstrip('/')
        self.assertEqual(pub.stat().st_mode & 0o777, 0o644)
        self.assertEqual((self.root / lifecycle.PRIVATE.lstrip('/')).stat().st_mode & 0o777, 0o600)

class PackagingTests(unittest.TestCase):
    def test_sixteen_variants_keep_cli_independent(self):
        import networkmanager_package as packaging
        import itertools
        source = Path(__file__).parent.resolve()
        for arch, fmt, channel, cli in itertools.product(('amd64', 'arm64'), ('deb', 'rpm'), ('public', 'internal'), (False, True)):
            with self.subTest(arch=arch, fmt=fmt, channel=channel, cli=cli), tempfile.TemporaryDirectory() as tmp:
                base = Path(tmp)
                binaries = base / 'binaries'
                staging = base / 'staging'
                component = binaries / 'networkmanager'
                component.mkdir(parents=True)
                if not cli:
                    for name in [*packaging.PAYLOAD, 'nm_aheadscale.pp', 'LICENSE', 'NOTICES.md', 'SOURCE.json']:
                        (component / name).write_bytes(b'pinned component fixture')
                root = source.parents[1]
                manifest = packaging.stage_networkmanager(root, binaries, staging, cli, fmt, channel)
                if cli:
                    self.assertEqual(manifest['files'], {})
                    self.assertFalse((staging / 'usr/libexec/nm-aheadscale-service').exists())
                else:
                    self.assertTrue((staging / 'usr/libexec/nm-aheadscale-service').exists())
                    self.assertTrue(any('NetworkManager' in name for name in manifest['files']))
                self.assertEqual(manifest['management_url'], 'https://aheadscale.ru:443' if channel == 'public' else 'https://macmini.tailb71cbe.ts.net:10443')
                hook = packaging.prepare_hook(root, manifest)
                self.assertIn("'prepare', 'cli'" if cli else "'prepare', 'desktop'", hook)
                if fmt == 'deb':
                    packaging.deb_hooks(root, staging, manifest)
                    self.assertTrue((staging / 'DEBIAN/preinst').stat().st_mode & 0o111)

if __name__ == '__main__': unittest.main()
