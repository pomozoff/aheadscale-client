#!/usr/bin/env python3
"""Выбранные CI-проверки; модель не участвует в выполнении отдельных шагов."""
import argparse
from dataclasses import dataclass, field
import json
import os
import platform
from pathlib import Path
import re
import signal
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
SUITES = ("ci-contract", "unit", "packaging", "networkmanager", "auth", "gtk-editors", "plasma")


@dataclass
class Check:
    id: str
    argv: list[str]
    timeout: float
    env: dict[str, str] = field(default_factory=dict)


def suite_checks(suite):
    go = ["go", "test", "-count=1"]
    if suite == "ci-contract":
        return [Check("runner-contract", [sys.executable, "-B", "-m", "unittest", "discover", "-s", "testing/aheadscale", "-p", "test_*.py", "-v"], 120)]
    if suite == "unit":
        return [
            Check("client-core", go + ["./client/" + p for p in ["cmd", "status", "internal/auth", "internal/templates", "internal/profilemanager", "internal/daemonaddr", "internal/selfroute", "internal/ipcauth", "net", "internal/routemanager/systemops", "configs"]], 900),
            Check("linux-dns-service", go + ["-race", "-tags", "devcert", "./client/internal/dns", "./client/internal", "./client/server"], 900, {"DBUS_SYSTEM_BUS_ADDRESS": "unix:path=/tmp/aheadscale-ci-no-system-bus"}),
            Check("updater-policy", go + ["./client/internal/updater", "-run", "TestAheadscale"], 300),
        ]
    if suite == "packaging":
        return [
            Check("go-packaging", go + ["-race", "./packaging/aheadscale"], 600),
            Check("python-packaging", [sys.executable, "-B", "-m", "unittest", "discover", "-s", "packaging/aheadscale", "-p", "test_*.py", "-v"], 180),
            Check("bash-syntax", ["bash", "-n", "packaging/aheadscale/build.sh", "packaging/aheadscale/scripts/macos-preinstall", "packaging/aheadscale/scripts/macos-postinstall", "packaging/aheadscale/scripts/macos-uninstall.command"], 30),
            Check("sh-syntax", ["sh", "-n", "packaging/aheadscale/scripts/postinst", "packaging/aheadscale/scripts/prerm", "packaging/aheadscale/scripts/cleanup-networkmanager"], 30),
            Check("diff-format", ["git", "diff", "--check"], 30),
        ]
    if suite == "plasma":
        return [Check("plasma-factory-and-dialog", [sys.executable, "-B", "testing/aheadscale/plasma.py", "--output", "ci-results/plasma-component"], 600)]
    if suite == "gtk-editors":
        return [Check("gtk-model-and-factories", [sys.executable, "-B", "testing/aheadscale/editors.py"], 300)]
    if suite == "auth":
        return [Check("registration-account-device-isolation", go + ["-race", "-tags", "devcert", "./management/server", "-run", "^TestAheadscale"], 1500)]
    if suite == "networkmanager":
        return [Check("native-controller", go + ["-race", "./client/internal/linuxvpn"], 600),
                Check("plugin-protocol-dbus", [sys.executable, "-B", "testing/aheadscale/networkmanager.py"], 900)]
    raise ValueError("Неизвестный набор проверок: " + suite)


def redact(value, secrets):
    for secret in sorted((s for s in secrets if s), key=len, reverse=True):
        value = value.replace(secret, "[REDACTED]")
    return value


def stop_process(process):
    if process.poll() is not None:
        return
    if os.name == "posix":
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    else:
        process.kill()
    process.wait()


def execute_checks(checks, output, source_sha, suite, secrets=()):
    if not checks or len({c.id for c in checks}) != len(checks):
        raise ValueError("Набор пуст или идентификаторы повторяются")
    for check in checks:
        if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", check.id) or not check.argv or check.timeout <= 0:
            raise ValueError("Некорректная проверка")
    output = Path(output)
    output.mkdir(parents=True, exist_ok=True)
    (output / "logs").mkdir(exist_ok=True)
    result = {"schemaVersion": 1, "sourceCommit": source_sha, "suite": suite,
              "status": "FAIL", "acceptance": "NOT_RUN", "platform": sys.platform,
              "architecture": platform.machine(), "runnerImage": os.environ.get("ImageVersion"),
              "plannedChecks": len(checks), "checks": []}
    for check in checks:
        env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
        for name in list(env):
            if name in ("GH_TOKEN", "GITHUB_TOKEN") or name.startswith(("AWS_", "AZURE_")):
                del env[name]
        env.update(check.env)
        start = time.monotonic()
        timed_out = False
        cancelled = False
        process = None
        try:
            process = subprocess.Popen(check.argv, cwd=ROOT, env=env,
                                       stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                       start_new_session=(os.name == "posix"))
            try:
                data, _ = process.communicate(timeout=check.timeout)
            except subprocess.TimeoutExpired:
                timed_out = True
                stop_process(process)
                data, _ = process.communicate()
            except KeyboardInterrupt:
                cancelled = True
                stop_process(process)
                data, _ = process.communicate()
            code = 130 if cancelled else process.returncode
        except OSError as exc:
            data, code = str(exc).encode(), 127
        finally:
            if process is not None:
                stop_process(process)
        text = redact(data.decode("utf-8", errors="replace"), secrets)
        log = "logs/" + check.id + ".log"
        (output / log).write_text(text)
        row = {"id": check.id, "status": "PASS" if code == 0 and not timed_out else "FAIL",
               "exitCode": code, "timedOut": timed_out, "cancelled": cancelled, "seconds": round(time.monotonic() - start, 3), "log": log}
        result["checks"].append(row)
        result["status"] = "PASS" if len(result["checks"]) == result["plannedChecks"] and all(c["status"] == "PASS" for c in result["checks"]) else "FAIL"
        write_reports(output, result)
        if cancelled:
            break
    return result


def write_reports(output, result):
    temp = output / "results.json.tmp"
    temp.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    temp.replace(output / "results.json")
    failures = sum(c["status"] != "PASS" for c in result["checks"])
    junit = ET.Element("testsuite", name=result["suite"], tests=str(len(result["checks"])), failures=str(failures))
    for check in result["checks"]:
        case = ET.SubElement(junit, "testcase", name=check["id"], time=str(check["seconds"]))
        if check["status"] != "PASS":
            ET.SubElement(case, "failure", message="cancelled" if check["cancelled"] else "timeout" if check["timedOut"] else "exit=" + str(check["exitCode"])).text = "Диагностика: " + check["log"]
    ET.ElementTree(junit).write(output / "junit.xml", encoding="unicode", xml_declaration=True)
    lines = ["## Aheadscale: " + result["suite"] + " — " + result["status"],
             "", "Исходники: `" + result["sourceCommit"] + "`", "",
             "Полная приёмка платформ/сценариев: **NOT_RUN**.", "",
             "| Проверка | Результат | Диагностика |", "| --- | --- | --- |"]
    lines += ["| " + c["id"] + " | " + c["status"] + " | " + c["log"] + " |" for c in result["checks"]]
    (output / "summary.md").write_text("\n".join(lines) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", required=True, choices=SUITES)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    sha = os.environ.get("GITHUB_SHA") or subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    secrets = json.loads(os.environ.get("AHEADSCALE_TEST_SECRET_VALUES", "[]"))
    if not isinstance(secrets, list) or any(not isinstance(s, str) for s in secrets):
        parser.error("Список скрываемых значений должен быть JSON-массивом строк")
    def interrupted(signum, frame):
        raise KeyboardInterrupt
    previous = signal.signal(signal.SIGTERM, interrupted)
    try:
        result = execute_checks(suite_checks(args.suite), args.output, sha, args.suite, secrets)
    finally:
        signal.signal(signal.SIGTERM, previous)
    print(result["suite"] + ": " + result["status"] + "; " + str(len(result["checks"])) + " проверок")
    if any(c["cancelled"] for c in result["checks"]):
        return 130
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
