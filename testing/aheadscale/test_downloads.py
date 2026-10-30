"""Проверки экспорта реальных файлов и отказа при подмене кандидата."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

MODULE = Path(__file__).resolve().parents[2] / 'packaging/aheadscale/downloads.py'
downloads = None
if MODULE.exists():
    spec = importlib.util.spec_from_file_location('downloads', MODULE)
    downloads = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(downloads)


class DownloadsTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.artifacts = self.root / 'artifacts'
        self.artifacts.mkdir()
        self.output = self.root / 'output'
        self.version = '0.80.0-aheadscale.33'
        self.registry = {'schema_version': 1, 'release_ready': False,
                         'client_version': self.version, 'source_commit': 'a' * 40, 'builds': []}
        for arch in ('amd64', 'arm64'):
            folder = self.artifacts / ('linux-' + arch)
            folder.mkdir()
            files = []
            for kind in ('public', 'public_cli'):
                for ext in ('deb', 'rpm'):
                    name = f'aheadscale_{self.version}_linux_{arch}_{kind}.{ext}'
                    data = (arch + kind + ext).encode()
                    (folder / name).write_bytes(data)
                    files.append({'name': name, 'sha256': hashlib.sha256(data).hexdigest(), 'size': len(data)})
            manifest = {'sourceCommit': 'a' * 40, 'clientVersion': self.version,
                        'platform': 'linux', 'architecture': arch, 'files': files, 'releaseReady': False,
                        'buildRun': 123 if arch == 'amd64' else 124}
            self.registry['builds'].append({'run': manifest['buildRun'], 'architecture': arch,
                                           'local_directory': str(folder), 'manifest': manifest})

    def export(self):
        self.assertIsNotNone(downloads, 'Экспорт каталога ещё не реализован')
        return downloads.export_preview(self.registry, self.artifacts, self.output)

    def first_file(self):
        b = self.registry['builds'][0]
        return Path(b['local_directory']) / b['manifest']['files'][0]['name']

    def test_exact_bytes_urls_provenance_and_checksums(self):
        path = self.export()
        manifest = json.loads((path / 'manifest.json').read_text())
        self.assertEqual(len(manifest['packages']), 8)
        self.assertEqual(manifest['channel'], 'preview')
        self.assertIs(manifest['releaseReady'], False)
        self.assertEqual(manifest['sourceCommit'], 'a' * 40)
        self.assertTrue(manifest['warning'])
        sums = (path / 'SHA256SUMS').read_text().splitlines()
        self.assertEqual(len(sums), 8)
        for p in manifest['packages']:
            relative = f"linux-{p['architecture']}/{p['name']}"
            data = (path / relative).read_bytes()
            self.assertEqual(hashlib.sha256(data).hexdigest(), p['sha256'])
            self.assertEqual(len(data), p['size'])
            self.assertEqual(p['url'], f'https://aheadscale.ru/downloads/preview/{self.version}/{relative}')
            self.assertIn(f"{p['sha256']}  {relative}", sums)
            self.assertIn(p['buildRun'], (123, 124))
        self.assertFalse((self.output / 'beta').exists())

    def test_tampered_bytes_or_size_never_publish_partial_tree(self):
        for what in ('bytes', 'size'):
            with self.subTest(what=what):
                if what == 'bytes':
                    p = self.first_file(); original = p.read_bytes(); p.write_bytes(b'X' * len(original))
                else:
                    self.registry['builds'][0]['manifest']['files'][0]['size'] += 1
                with self.assertRaises(ValueError):
                    self.export()
                self.assertFalse((self.output / 'preview' / self.version).exists())
                if what == 'bytes':
                    p.write_bytes(original)

    def test_mismatched_version_architecture_commit_or_run_rejected(self):
        changes = [('clientVersion', 'other'), ('architecture', 'arm64'),
                   ('sourceCommit', 'b' * 40), ('platform', 'windows'), ('buildRun', 999)]
        for key, value in changes:
            with self.subTest(key=key):
                original = copy.deepcopy(self.registry)
                self.registry['builds'][0]['manifest'][key] = value
                with self.assertRaises(ValueError):
                    self.export()
                self.registry = original

    def test_internal_missing_duplicate_or_unsafe_filename_rejected(self):
        files = self.registry['builds'][0]['manifest']['files']
        for name in ('../outside.deb', '/etc/passwd', 'package_internal.deb'):
            with self.subTest(name=name):
                original = files[0]['name']; files[0]['name'] = name
                with self.assertRaises(ValueError):
                    self.export()
                files[0]['name'] = original
        files.append(copy.deepcopy(files[0]))
        with self.assertRaises(ValueError):
            self.export()
        files.pop(); files.pop()
        with self.assertRaises(ValueError):
            self.export()

    def test_symlink_package_and_outside_root_rejected(self):
        p = self.first_file(); data = p.read_bytes(); p.unlink()
        target = self.root / 'outside'; target.write_bytes(data); p.symlink_to(target)
        with self.assertRaises(ValueError):
            self.export()
        p.unlink(); p.write_bytes(data)
        self.registry['builds'][0]['local_directory'] = str(self.root)
        with self.assertRaises(ValueError):
            self.export()

    def test_reexport_does_not_overwrite_existing_version(self):
        path = self.export()
        before = (path / 'manifest.json').read_bytes()
        with self.assertRaises(FileExistsError):
            self.export()
        self.assertEqual((path / 'manifest.json').read_bytes(), before)

    def test_invalid_or_duplicate_build_rejected(self):
        self.registry['builds'].append(copy.deepcopy(self.registry['builds'][0]))
        with self.assertRaises(ValueError):
            self.export()
        self.registry['builds'] = []
        with self.assertRaises(ValueError):
            self.export()

    def test_invalid_digest_size_and_version_rejected(self):
        for key, value in (('sha256', 'x' * 64), ('size', -1), ('size', True)):
            original = copy.deepcopy(self.registry)
            self.registry['builds'][0]['manifest']['files'][0][key] = value
            with self.assertRaises(ValueError):
                self.export()
            self.registry = original
        self.registry['client_version'] = '../escape'
        with self.assertRaises(ValueError):
            self.export()


if __name__ == '__main__':
    unittest.main()
