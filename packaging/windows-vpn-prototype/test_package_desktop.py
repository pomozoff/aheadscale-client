"""Проверки реальных preview-установщиков без запуска Windows-кода."""
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import unittest
import zipfile


ROOT = Path(__file__).resolve().parents[2]
PACKAGER = ROOT / "packaging/windows-vpn-prototype/package-desktop.py"


class DesktopPackageTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="aheadscale-package-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def inputs(self, arch):
        native = self.root / "native"
        standard = self.root / "standard"
        native.mkdir()
        standard.mkdir()
        machine = {"amd64": 0x8664, "arm64": 0xAA64}[arch]
        for folder, names in ((native, ["AheadscaleNativeUI.exe"]),
                              (standard, ["aheadscale.exe", "aheadscale-ui.exe"])):
            for name in names:
                binary = bytearray(128)
                binary[:2] = b"MZ"
                struct.pack_into("<I", binary, 60, 64)
                binary[64:68] = b"PE\0\0"
                struct.pack_into("<H", binary, 68, machine)
                (folder / name).write_bytes(binary)
        package = native / "Aheadscale.VpnPrototype.msix"
        with zipfile.ZipFile(package, "w") as archive:
            archive.writestr("AppxManifest.xml", '<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Identity ProcessorArchitecture="' + ("x64" if arch == "amd64" else "arm64") + '"/></Package>')
        (native / "Aheadscale.VpnPrototype.cer").write_bytes(b"test certificate")
        def sha(path):
            return hashlib.sha256(path.read_bytes()).hexdigest().upper()
        common = {"architecture": arch, "clientVersion": "0.80.0-aheadscale.23", "sourceCommit": "a" * 40}
        self.native_manifest = dict(common, packageFile=package.name, sha256=sha(package),
                                    nativeUIFile="AheadscaleNativeUI.exe",
                                    nativeUISha256=sha(native / "AheadscaleNativeUI.exe"))
        self.standard_manifest = dict(common, cliSha256=sha(standard / "aheadscale.exe"),
                                      uiSha256=sha(standard / "aheadscale-ui.exe"))
        self.native = native
        self.standard = standard
        self.save_manifests()
        self.build_standard()

    def build_standard(self):
        standard = self.standard
        script = self.root / "standard.nsi"
        script.write_text('OutFile "' + str(self.root / "standard.exe") + '"\nSection\nSetOutPath "$INSTDIR"\nFile "' + str(standard / "aheadscale.exe") + '"\nFile "' + str(standard / "aheadscale-ui.exe") + '"\nSectionEnd\n')
        subprocess.run(["makensis", "-V1", str(script)], check=True, capture_output=True)

    def save_manifests(self):
        (self.native / "build-manifest.json").write_text(json.dumps(self.native_manifest))
        (self.standard / "build-manifest.json").write_text(json.dumps(self.standard_manifest))

    def package(self):
        return subprocess.run([sys.executable, str(PACKAGER), "--native", str(self.native),
                               "--standard-installer", str(self.root / "standard.exe"),
                               "--standard-cli", str(self.standard / "aheadscale.exe"),
                               "--standard-ui", str(self.standard / "aheadscale-ui.exe"),
                               "--output", str(self.root / "desktop.exe")],
                              capture_output=True, text=True)

    def test_arm64_installer_preserves_native_payload(self):
        self.inputs("arm64")
        result = self.package()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        extracted = self.root / "extracted"
        subprocess.run(["7z", "x", "-y", "-o" + str(extracted), str(self.root / "desktop.exe")],
                       check=True, capture_output=True)
        matches = list(extracted.rglob("AheadscaleNativeUI.exe"))
        self.assertEqual(len(matches), 1)
        self.assertEqual(matches[0].read_bytes(), (self.native / "AheadscaleNativeUI.exe").read_bytes())

    def test_amd64_installer_still_builds(self):
        self.inputs("amd64")
        result = self.package()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_mixed_standard_architecture_is_rejected(self):
        self.inputs("amd64")
        path = self.standard / "aheadscale.exe"
        binary = bytearray(path.read_bytes())
        struct.pack_into("<H", binary, 68, 0xAA64)
        path.write_bytes(binary)
        self.standard_manifest["cliSha256"] = hashlib.sha256(binary).hexdigest().upper()
        self.save_manifests()
        self.build_standard()
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("архитектур", result.stderr)
        self.assertFalse((self.root / "desktop.exe").exists())

    def test_different_source_commit_is_rejected(self):
        self.inputs("amd64")
        self.standard_manifest["sourceCommit"] = "b" * 40
        self.save_manifests()
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("исходник", result.stderr)

    def test_different_client_version_is_rejected(self):
        self.inputs("amd64")
        self.standard_manifest["clientVersion"] = "0.80.0-aheadscale.14"
        self.save_manifests()
        result = self.package()
        self.assertNotEqual(result.returncode, 0)

    def test_mixed_native_ui_architecture_is_rejected(self):
        self.inputs("amd64")
        path = self.native / "AheadscaleNativeUI.exe"
        binary = bytearray(path.read_bytes())
        struct.pack_into("<H", binary, 68, 0xAA64)
        path.write_bytes(binary)
        self.native_manifest["nativeUISha256"] = hashlib.sha256(binary).hexdigest().upper()
        self.save_manifests()
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("архитектур", result.stderr)

    def test_mixed_msix_architecture_is_rejected(self):
        self.inputs("amd64")
        path = self.native / self.native_manifest["packageFile"]
        with zipfile.ZipFile(path, "w") as archive:
            archive.writestr("AppxManifest.xml", '<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Identity ProcessorArchitecture="arm64"/></Package>')
        self.native_manifest["sha256"] = hashlib.sha256(path.read_bytes()).hexdigest().upper()
        self.save_manifests()
        result = self.package()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("архитектур", result.stderr)


if __name__ == "__main__":
    unittest.main()
