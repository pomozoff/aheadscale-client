#!/usr/bin/env python3
"""Временный native gate XPC в macOS-ВМ; только подписанные synthetic probes."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import sys
import tempfile
import time

SERVICE = "ru.aheadscale.probe.20261110"
TEAM = "Y24464MUL3"


def command(argv, check=True):
    result = subprocess.run(argv, capture_output=True, text=True, timeout=20)
    if check and result.returncode:
        raise RuntimeError("{}: {}".format(argv[0], result.stderr.strip()))
    return result


def events(path):
    if not path.exists():
        return []
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


class Fixture:
    def __init__(self, binaries):
        self.binaries = binaries
        self.root = Path(tempfile.mkdtemp(prefix="aheadscale-bridge-native-", dir=str(binaries.parent)))
        self.root.chmod(0o700)
        self.domain = "gui/{}".format(os.getuid())
        self.owned = []

    def target(self, label):
        return self.domain + "/" + label

    def start(self, label, executable, arguments, output):
        target = self.target(label)
        if command(["launchctl", "print", target], check=False).returncode == 0:
            raise RuntimeError("Уже существует job {}; чужую job не заменяем".format(target))
        config = {
            "Label": label,
            "ProgramArguments": [str(self.binaries / executable)] + arguments,
            "RunAtLoad": True,
            "KeepAlive": False,
            "StandardOutPath": str(self.root / (output + ".stdout")),
            "StandardErrorPath": str(self.root / (output + ".stderr")),
        }
        if label == SERVICE:
            config["MachServices"] = {SERVICE: True}
        path = self.root / (label + ".plist")
        path.write_bytes(plistlib.dumps(config))
        path.chmod(0o600)
        self.owned.append(label)
        command(["sudo", "-n", "launchctl", "bootstrap", self.domain, str(path)])

    def stop(self, label):
        state = command(["launchctl", "print", self.target(label)], check=False)
        if state.returncode == 0:
            if str(self.root) not in state.stdout:
                raise RuntimeError("Не совпал путь владения job {}; удаление запрещено".format(label))
            command(["sudo", "-n", "launchctl", "bootout", self.target(label)])
        if label in self.owned:
            self.owned.remove(label)

    def start_server(self, executable, output):
        self.start(SERVICE, executable, ["server", "--seconds", "600"], output)
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            ready = [event for event in events(self.root / (output + ".stdout"))
                     if event.get("event") == "server_started"]
            if ready:
                if ready[-1].get("service") != SERVICE:
                    raise RuntimeError("Не совпало имя Mach service")
                state = command(["launchctl", "print", self.target(SERVICE)])
                if not re.search(r"\bpid = \d+", state.stdout):
                    raise RuntimeError("Server не работает")
                return
            time.sleep(2)
        raise RuntimeError("Server не сообщил готовность")

    def client(self, executable, case, refusal=False):
        label = SERVICE + ".client-" + case
        arguments = ["client", "--seconds", "120"]
        if refusal:
            arguments += ["--expect-refusal"]
        self.start(label, executable, arguments, case)
        deadline = time.monotonic() + (30 if refusal else 155)
        while time.monotonic() < deadline:
            state = command(["launchctl", "print", self.target(label)])
            match = re.search(r"last exit code = (\d+)", state.stdout)
            if match:
                if int(match.group(1)) != 0:
                    raise RuntimeError("Client {} завершился с ошибкой; см. локальные stdout/stderr".format(case))
                output = events(self.root / (case + ".stdout"))
                expected = "refused" if refusal else "pass"
                results = [item for item in output if item.get("event") == expected]
                if len(results) != 1:
                    raise RuntimeError("Нет единственного результата {}".format(case))
                result = results[0]
                if not refusal and (result.get("seconds", 0) < 120 or result.get("ipv4", 0) <= 0
                                    or result.get("ipv6", 0) <= 0
                                    or result.get("frames") != result.get("ipv4", 0) + result.get("ipv6", 0)):
                    raise RuntimeError("Недостаточный двухминутный IPv4/IPv6 прогон")
                self.stop(label)
                return result
            if "last terminating signal" in state.stdout:
                raise RuntimeError("Client {} остановлен сигналом".format(case))
            time.sleep(5)
        raise RuntimeError("Client {} не завершился в срок".format(case))

    def received(self, output):
        return sum(item.get("event") == "descriptor_received"
                   for item in events(self.root / (output + ".stdout")))

    def cleanup(self):
        errors = []
        for label in list(reversed(self.owned)):
            try:
                self.stop(label)
            except Exception as error:
                errors.append(str(error))
        return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--expected-source", required=True)
    parser.add_argument("--untrusted-sha256", required=True)
    args = parser.parse_args()
    if sys.platform != "darwin" or os.geteuid() == 0:
        raise RuntimeError("Запускается обычным пользователем только в изолированной macOS-ВМ")
    binaries = args.input.resolve()
    if (binaries / "source.txt").read_text().strip() != args.expected_source:
        raise RuntimeError("Не совпала версия проб")
    raw = binaries / "probe-no-developerid"
    if hashlib.sha256(raw.read_bytes()).hexdigest() != args.untrusted_sha256:
        raise RuntimeError("Не совпала исходная CI-проба без Developer ID")
    for role in ("server", "client", "wrong"):
        requirement = 'anchor apple generic and certificate leaf[subject.OU] = "{}" and identifier "ru.aheadscale.probe.{}"'.format(TEAM, role)
        command(["codesign", "--verify", "--strict", "-R", requirement, str(binaries / ("probe-" + role))])
    command(["sudo", "-n", "true"])
    fixture = Fixture(binaries)
    report = {"source": args.expected_source, "result": "fail", "cases": {}}
    try:
        fixture.start_server("probe-server", "server-positive")
        report["cases"]["duplex-120s"] = fixture.client("probe-client", "positive")
        if fixture.received("server-positive") != 1:
            raise RuntimeError("Положительный server не принял ровно один дескриптор")
        fixture.stop(SERVICE)
        fixture.start_server("probe-server", "server-untrusted-clients")
        report["cases"]["no-developerid"] = fixture.client("probe-no-developerid", "adhoc", refusal=True)
        report["cases"]["wrong-client-id"] = fixture.client("probe-wrong", "wrong-client", refusal=True)
        if fixture.received("server-untrusted-clients") != 0:
            raise RuntimeError("Неверному клиенту был передан канал")
        fixture.stop(SERVICE)
        fixture.start_server("probe-wrong", "server-wrong-id")
        report["cases"]["wrong-server-id"] = fixture.client("probe-client", "wrong-server", refusal=True)
        if fixture.received("server-wrong-id") != 0:
            raise RuntimeError("Неверный server получил дескриптор")
        report["result"] = "pass"
    except Exception as error:
        report["error"] = str(error)
    finally:
        cleanup = fixture.cleanup()
        if cleanup:
            report["result"] = "fail"
            report["cleanup_errors"] = cleanup
        (fixture.root / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({"result": report["result"], "evidence": str(fixture.root / "result.json")}))
    return 0 if report["result"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
