"""Границы доверия между KDE producer и сборкой установщика."""
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class PlasmaContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        path = ROOT / 'testing/aheadscale/plasma.py'
        if not path.is_file():
            raise AssertionError('Не реализован проверяемый KDE producer')
        spec = importlib.util.spec_from_file_location('plasma_builder', path)
        cls.builder = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.builder)
        cls.pin = json.loads((ROOT/'packaging/aheadscale/networkmanager-source.json').read_text())

    def test_corrupt_sdk_does_not_become_a_header(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'vpnuiplugin.h'
            with self.assertRaises(ValueError):
                self.builder.write_verified(p,b'corrupted header','0'*64)
            self.assertFalse(p.exists())

    def test_valid_sdk_is_saved_only_after_hash_check(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'header.h'; body=b'synthetic header\n'
            self.builder.write_verified(p,body,hashlib.sha256(body).hexdigest())
            self.assertEqual(p.read_bytes(),body)

    def test_wrong_os_arch_or_toolchain_is_rejected(self):
        good=dict(self.pin['plasma_toolchain'],architecture='amd64')
        self.builder.validate_environment(self.pin,good)
        for key,value in [('os','Ubuntu 24.04'),('architecture','mips64'),('qt','6.9.0'),
                          ('kf','6.20.0'),('plasma','6.5.0'),('cmake','3.28.3'),('gcc','14.2.0')]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.builder.validate_environment(self.pin,good|{key:value})

    def test_component_requires_native_64_bit_shared_elf(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'plugin.so'
            header=bytearray(64);header[:7]=b'\x7fELF\x02\x01\x01';struct.pack_into('<HH',header,16,3,62)
            p.write_bytes(header)
            self.builder.check_component(p,'amd64')
            with self.assertRaises(ValueError):self.builder.check_component(p,'arm64')
            for changed in (b'not an ELF',bytes(header[:4])+b'\x01'+bytes(header[5:])):
                p.write_bytes(changed)
                with self.assertRaises(ValueError):self.builder.check_component(p,'amd64')

    def test_unpinned_candidate_cannot_enter_installer(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'plugin.so';p.write_bytes(b'candidate')
            pin=dict(self.pin,plasma_component_sha256={})
            with self.assertRaises(ValueError):self.builder.verify_provenance(pin,{},p,'amd64')

    def test_foreign_provenance_and_changed_blob_are_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'plugin.so'
            header=bytearray(64);header[:7]=b'\x7fELF\x02\x01\x01';struct.pack_into('<HH',header,16,3,62)
            p.write_bytes(header);digest=hashlib.sha256(header).hexdigest()
            pin=dict(self.pin,plasma_component_sha256={'amd64':digest})
            meta={k:self.pin[k] for k in ('upstream_commit','adapted_commit','archive_sha256','patch_sha256','plasma_sdk','plasma_toolchain')}
            meta.update(architecture='amd64',component_sha256=digest,client_revision='1'*40)
            self.builder.verify_provenance(pin,meta,p,'amd64')
            with self.assertRaises(ValueError):
                self.builder.verify_provenance(pin|{'expected_producer_revision':'2'*40},meta,p,'amd64')
            for changed in ({'adapted_commit':'0'*40},{'architecture':'arm64'},
                            {'component_sha256':'0'*64},{'client_revision':'latest'}):
                with self.subTest(changed=changed),self.assertRaises(ValueError):
                    self.builder.verify_provenance(pin,meta|changed,p,'amd64')
            p.write_bytes(bytes(header)+b'tampered')
            with self.assertRaises(ValueError):self.builder.verify_provenance(pin,meta,p,'amd64')


class ProducerRunContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec=importlib.util.spec_from_file_location('producer',ROOT/'testing/aheadscale/plasma.py')
        cls.builder=importlib.util.module_from_spec(spec);spec.loader.exec_module(cls.builder)

    def test_only_completed_own_producer_workflow_is_accepted(self):
        self.assertTrue(hasattr(self.builder,'validate_run'),'Нет проверки выбранного Actions run')
        good={'id':123,'status':'completed','conclusion':'success','event':'workflow_dispatch',
              'path':'.github/workflows/aheadscale-client.yml','head_sha':'a'*40,
              'head_repository':{'full_name':'pomozoff/aheadscale-client'}}
        self.assertEqual(self.builder.validate_run(good,'123'),'a'*40)
        for changed in ({'id':124},{'status':'in_progress'},{'conclusion':'failure'},
                        {'path':'.github/workflows/foreign.yml'},{'event':'pull_request'},
                        {'head_sha':'latest'},{'head_repository':{'full_name':'foreign/fork'}}):
            with self.subTest(changed=changed),self.assertRaises(ValueError):
                self.builder.validate_run(good|changed,'123')
        for bad in ('0','-1','123;whoami'):
            with self.subTest(bad=bad),self.assertRaises(ValueError):self.builder.validate_run(good,bad)
