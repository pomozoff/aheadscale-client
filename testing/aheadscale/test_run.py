"""Контракт CI: настоящий дочерний процесс, без запуска продуктовых наборов."""
import importlib.util
import json
import os
import signal
import time
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET

spec = importlib.util.spec_from_file_location("aheadscale_ci", Path(__file__).with_name("run.py"))
ci = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = ci
spec.loader.exec_module(ci)


class RunnerContract(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.out = Path(self.temp.name)

    def execute(self, checks, **kw):
        return ci.execute_checks(checks, self.out, source_sha="a" * 40, suite="contract", **kw)

    def test_success_has_source_summary_and_junit(self):
        result = self.execute([ci.Check("pass", [sys.executable, "-c", "print('done')"], 3)])
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["sourceCommit"], "a" * 40)
        self.assertEqual(json.loads((self.out / "results.json").read_text())["status"], "PASS")
        self.assertEqual(ET.parse(self.out / "junit.xml").getroot().get("failures"), "0")
        self.assertIn("PASS", (self.out / "summary.md").read_text())
        self.assertEqual(result["acceptance"], "NOT_RUN")

    def test_failure_cannot_be_hidden_by_later_success(self):
        result = self.execute([
            ci.Check("bad", [sys.executable, "-c", "raise SystemExit(7)"], 3),
            ci.Check("good", [sys.executable, "-c", "pass"], 3),
        ])
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["checks"][0]["exitCode"], 7)
        self.assertEqual(result["checks"][1]["status"], "PASS")
        self.assertEqual(ET.parse(self.out / "junit.xml").getroot().get("failures"), "1")

    def test_timeout_is_failed_and_report_survives(self):
        result = self.execute([ci.Check("slow", [sys.executable, "-c", "import time;time.sleep(60)"], .2)])
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(result["checks"][0]["timedOut"])
        self.assertTrue((self.out / "results.json").is_file())

    @unittest.skipUnless(os.name == "posix", "Сигнальный контракт Unix runner")
    def test_interrupted_process_is_reported_and_suite_never_looks_complete(self):
        marker = self.out / "ready"
        child = "import os,pathlib,time;pathlib.Path(" + repr(str(marker)) + ").write_text(str(os.getpid()));time.sleep(60)"
        program = (
            "import importlib.util,sys,pathlib;"
            "s=importlib.util.spec_from_file_location('ci'," + repr(str(Path(__file__).with_name("run.py"))) + ");"
            "ci=importlib.util.module_from_spec(s);sys.modules[s.name]=ci;s.loader.exec_module(ci);"
            "r=ci.execute_checks([ci.Check('first',[sys.executable,'-c','pass'],3),"
            "ci.Check('pending',[sys.executable,'-c'," + repr(child) + "],30)],"
            "pathlib.Path(" + repr(str(self.out)) + "),'a'*40,'contract');"
            "raise SystemExit(0 if r['status']=='PASS' else 1)"
        )
        process = subprocess.Popen([sys.executable, "-c", program], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        def cleanup():
            if process.poll() is None:
                process.kill()
            process.wait()
            if marker.exists():
                pid = int(marker.read_text())
                cmdline = Path('/proc') / str(pid) / 'cmdline'
                if cmdline.exists() and str(marker).encode() in cmdline.read_bytes():
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
        self.addCleanup(cleanup)
        deadline = time.monotonic() + 10
        while not marker.exists() and process.poll() is None and time.monotonic() < deadline:
            time.sleep(.02)
        self.assertTrue(marker.exists(), "Дочерний процесс не запустился")
        partial = json.loads((self.out / 'results.json').read_text())
        self.assertEqual(partial['status'], 'FAIL')
        process.send_signal(signal.SIGINT)
        self.assertNotEqual(process.wait(timeout=5), 0)
        final = json.loads((self.out / 'results.json').read_text())
        self.assertEqual(final['status'], 'FAIL')
        self.assertEqual(len(final['checks']), 2)
        self.assertTrue(final['checks'][-1]['cancelled'])

    def test_missing_program_is_reported_as_failure(self):
        result = self.execute([ci.Check("missing", [str(self.out / "absent")], 1)])
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["checks"][0]["exitCode"], 127)

    def test_secrets_redacted_from_all_published_files(self):
        secret = "synthetic-password-not-for-logs"
        result = self.execute([ci.Check("redact", [sys.executable, "-c", "print(" + repr(secret) + ")"], 3)], secrets=[secret])
        self.assertEqual(result["status"], "PASS")
        for path in self.out.rglob("*"):
            if path.is_file():
                self.assertNotIn(secret, path.read_text(), path.name)
        self.assertIn("[REDACTED]", (self.out / "logs/redact.log").read_text())

    def test_empty_suite_is_not_success(self):
        with self.assertRaises(ValueError):
            self.execute([])

    def test_output_path_cannot_escape_via_check_id(self):
        with self.assertRaises(ValueError):
            self.execute([ci.Check("../escape", [sys.executable, "-c", "pass"], 1)])
        self.assertFalse((self.out.parent / "escape.log").exists())

    def test_suite_commands_are_argument_lists_and_ids_unique(self):
        for name in ci.SUITES:
            checks = ci.suite_checks(name)
            self.assertTrue(checks)
            self.assertEqual(len(checks), len({c.id for c in checks}))
            self.assertTrue(all(isinstance(c.argv, list) and c.timeout > 0 for c in checks))
        with self.assertRaises(ValueError):
            ci.suite_checks("unknown; touch /tmp/should-not-exist")

    def test_cli_rejects_unknown_suite_without_executing_it(self):
        result = subprocess.run([sys.executable, str(Path(__file__).with_name("run.py")), "--suite", "unknown", "--output", str(self.out)], capture_output=True)
        self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    unittest.main()
