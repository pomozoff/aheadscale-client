#!/usr/bin/env python3
"""Проверенный исходник адаптера и его настоящие Go/D-Bus проверки."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def main():
    if hasattr(os, "geteuid") and os.geteuid() == 0:
        raise SystemExit("Проверки D-Bus должны выполняться обычным CI-пользователем")
    with tempfile.TemporaryDirectory(prefix="aheadscale-ci-nm-") as directory:
        work = Path(directory)
        subprocess.run([sys.executable, str(ROOT / "packaging/aheadscale/networkmanager/prepare-source.py"), str(ROOT), str(work)], check=True)
        source = work / "source"
        subprocess.run(["git", "apply", "--unidiff-zero", str(ROOT / "packaging/aheadscale/networkmanager/aheadscale.patch")], cwd=source, check=True)
        # Отсутствующий независимый fixture обязан дать FAIL, не Skip.
        subprocess.run(["go", "test", "-race", "./internal/...", "-count=1"], cwd=source, check=True)


if __name__ == "__main__":
    main()
