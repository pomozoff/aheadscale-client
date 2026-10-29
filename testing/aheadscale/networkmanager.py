#!/usr/bin/env python3
"""Проверенный исходник адаптера и его настоящие Go/D-Bus проверки."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]


def main():
    if hasattr(os, "geteuid") and os.geteuid() == 0:
        raise SystemExit("Проверки D-Bus должны выполняться обычным CI-пользователем")
    with tempfile.TemporaryDirectory(prefix="aheadscale-ci-nm-") as directory:
        work = Path(directory)
        subprocess.run([sys.executable, str(ROOT / "packaging/aheadscale/networkmanager/prepare-source.py"), str(ROOT), str(work)], check=True)
        source = work / "source"
        subprocess.run(["git", "apply", "--unidiff-zero", str(ROOT / "packaging/aheadscale/networkmanager/aheadscale.patch")], cwd=source, check=True)
        binary = work / "daemon-fixture"
        subprocess.run(["go", "build", "-buildvcs=false", "-o", str(binary), "./testing/aheadscale/nm-fixture"], cwd=ROOT, check=True)
        socket = work / "daemon.sock"
        helper = subprocess.Popen([str(binary), "--socket", str(socket)])
        try:
            deadline = time.monotonic() + 10
            while not socket.exists():
                if helper.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError("Независимый fixture не запустился")
                time.sleep(.05)
            env = dict(os.environ, AHEADSCALE_COMPAT_DAEMON="unix://" + str(socket))
            subprocess.run(["go", "test", "-race", "./internal/...", "-count=1"], cwd=source, env=env, check=True)
        finally:
            helper.terminate()
            try:
                helper.wait(timeout=5)
            except subprocess.TimeoutExpired:
                helper.kill()
                helper.wait()
                raise RuntimeError("Fixture пришлось принудительно завершить при очистке")


if __name__ == "__main__":
    main()
