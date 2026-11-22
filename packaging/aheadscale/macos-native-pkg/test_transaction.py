"""Exercise the Installer shell transaction against an isolated fake target."""
import os
import plistlib
import shutil
import socket
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent
LABEL = "Y24464MUL3.ru.aheadscale.client.bridge"


class TransactionTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory(dir="/tmp", prefix="aspkg-")
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        root = self.root
        scripts = root / "scripts"
        scripts.mkdir()
        tools = root / "tools"
        tools.mkdir()
        self.jobs = root / "jobs"
        self.jobs.mkdir()
        common = (SCRIPTS / "installer-common.sh").read_text()
        common = common.replace("export PATH=/usr/bin", f"export PATH={tools}:/usr/bin")
        # Only the test copy can be redirected; production hooks have no
        # environment override for their target root or command search path.
        for prefix in ("/Applications", "/Library", "/usr/local", "/var"):
            for assignment in ("=", '=\"'):
                common = common.replace(assignment + prefix, assignment + str(root) + prefix)
        (scripts / "installer-common.sh").write_text(common)
        for name in ("preinstall", "postinstall"):
            source = (SCRIPTS / name).read_text()
            source = source.replace("/Applications /Library/LaunchDaemons /Library/PrivilegedHelperTools /usr/local/bin",
                                    " ".join(str(root) + p for p in (
                                        "/Applications", "/Library/LaunchDaemons",
                                        "/Library/PrivilegedHelperTools", "/usr/local/bin")))
            (scripts / name).write_text(source)
        self.scripts = scripts
        self.app = root / "Applications/Aheadscale.app"
        self.state = root / "var/lib/aheadscale"
        self.legacy = root / "Library/LaunchDaemons/aheadscale.plist"
        self.native = self.legacy.with_name(LABEL + ".plist")
        self.engine = root / "Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine"
        self.cli = root / "usr/local/bin/aheadscale"
        self.agent = self.app / "Contents/MacOS/aheadscale"
        self.backup = root / "var/db/aheadscale-native-install"
        self.backup.parent.mkdir(parents=True)
        self.sock = root / "var/run/aheadscale-native-prototype.sock"
        self.sock.parent.mkdir(parents=True)
        self.sock_handle = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.addCleanup(self.sock_handle.close)
        self.sock_handle.bind(str(self.sock))
        self.tool("stat", '''out=$(/usr/bin/stat "$@") || exit
if [[ "$2" = '%u %Lp' ]]; then printf '0 %s\\n' "${out##* }"; else printf '%s\\n' "$out"; fi''')
        for tool, body in {
            "uname": "echo arm64", "sw_vers": "echo 26.7", "codesign": "exit 0",
            "sleep": "exit 0", "ps": "exit 0", "perl": 'exit "$TEST_PROBE_EXIT"',
            "launchctl": '''case "$1" in
  print) test -f "$TEST_JOBS/${2#system/}" ;;
  bootstrap) touch "$TEST_JOBS/$(/usr/libexec/PlistBuddy -c 'Print :Label' "$3")" ;;
  bootout) rm -f "$TEST_JOBS/${2#system/}" ;;
  *) exit 1 ;;
esac''',
        }.items():
            self.tool(tool, body)

    def tool(self, name, body):
        target = self.root / "tools" / name
        target.write_text("#!/bin/bash\n" + body + "\n")
        target.chmod(0o755)

    @staticmethod
    def put(path, body=b"", mode=0o644):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(body)
        path.chmod(mode)

    def app_info(self, executable):
        self.put(self.app / "Contents/Info.plist", plistlib.dumps({
            "CFBundleIdentifier": "ru.aheadscale.client", "CFBundleExecutable": executable,
        }))

    def make_legacy(self):
        self.app_info("aheadscale-ui")
        self.put(self.agent, b"former executable", 0o755)
        self.put(self.legacy, plistlib.dumps({
            "Label": "aheadscale", "ProgramArguments": [str(self.agent)],
        }))
        self.put(self.jobs / "aheadscale")
        self.put(self.state / "default.json", b"old registration", 0o600)
        self.cli.parent.mkdir(parents=True, exist_ok=True)
        self.cli.symlink_to(self.agent)

    def unpack_native(self):
        if self.app.exists():
            shutil.rmtree(self.app)
        self.app_info("AheadscaleExtensionSetup")
        self.put(self.agent, b"new executable", 0o755)
        self.put(self.app / "Contents/Library/LaunchServices/AheadscaleNativeEngineBridge", b"bridge", 0o755)
        self.put(self.engine, b"engine", 0o755)
        plist = plistlib.loads((SCRIPTS / "launchd.plist").read_bytes())
        plist["ProgramArguments"][0] = str(self.app / "Contents/Library/LaunchServices/AheadscaleNativeEngineBridge")
        self.put(self.native, plistlib.dumps(plist))

    def run_hook(self, phase, probe=0):
        return subprocess.run(
            ["/bin/bash", str(self.scripts / phase)],
            env=dict(os.environ, TEST_PROBE_EXIT=str(probe), TEST_JOBS=str(self.jobs)),
            text=True, capture_output=True, timeout=20,
        )

    def succeeds(self, phase):
        result = self.run_hook(phase)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_clean_install_and_reinstall(self):
        self.succeeds("preinstall")
        self.unpack_native()
        self.succeeds("postinstall")
        self.assertEqual(self.cli.readlink(), self.agent)
        self.assertTrue((self.jobs / LABEL).exists())
        self.assertFalse(self.backup.exists())
        self.put(self.state / "default.json", b"new registration", 0o600)
        self.succeeds("preinstall")
        self.unpack_native()
        self.succeeds("postinstall")
        self.assertEqual((self.state / "default.json").read_bytes(), b"new registration")

    def test_failed_native_start_restores_legacy_and_registration(self):
        self.make_legacy()
        self.succeeds("preinstall")
        self.assertFalse((self.jobs / "aheadscale").exists())
        self.unpack_native()
        self.put(self.state / "default.json", b"failed new write", 0o600)
        result = self.run_hook("postinstall", probe=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("восстанавливается", result.stderr)
        self.assertEqual(self.agent.read_bytes(), b"former executable")
        self.assertEqual((self.state / "default.json").read_bytes(), b"old registration")
        self.assertTrue((self.jobs / "aheadscale").exists())
        self.assertFalse((self.jobs / LABEL).exists())
        self.assertFalse(self.native.exists())
        self.assertFalse(self.backup.exists())

    def test_collision_refuses_before_stopping_legacy(self):
        self.make_legacy()
        self.put(self.cli.parent / "foreign", b"foreign")
        self.cli.unlink()
        self.cli.symlink_to(self.cli.parent / "foreign")
        result = self.run_hook("preinstall")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue((self.jobs / "aheadscale").exists())
        self.assertFalse(self.backup.exists())
        self.assertEqual(self.cli.resolve().read_bytes(), b"foreign")


if __name__ == "__main__":
    unittest.main()
