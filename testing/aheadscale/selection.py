"""Выбор существующих CI-наборов; сборки разрешаются отдельно в workflow."""
import json
import os
from pathlib import Path
import subprocess


CHECKS = ("unit", "packaging", "auth", "windows")
TARGETS = ("current", "all", "arm64", "linux-amd64", "linux-arm64",
           "windows-amd64", "windows-arm64", "macos-arm64")


def select_checks(event, suite, targets, paths):
    selected = set()
    if event == "workflow_dispatch":
        if suite in CHECKS:
            selected.add(suite)
        elif suite == "email":
            selected.add("auth")
        elif suite in ("checks", "full"):
            selected.update(CHECKS)
        elif suite == "build":
            if targets not in TARGETS:
                raise ValueError("Неизвестная платформа сборки: " + targets)
            selected.update(("unit", "packaging"))
            if targets.startswith("windows-") or targets in ("current", "all", "arm64"):
                selected.add("windows")
        elif suite not in ("ci-contract", "server", "networkmanager", "gtk-editors",
                           "plasma", "plasma-input", "policy"):
            raise ValueError("Неизвестный набор: " + suite)
    elif event == "pull_request":
        for path in paths:
            if (path.endswith(".md") or path.startswith(("docs/", ".github/", "testing/aheadscale/"))
                    or path in ("LICENSE", "NOTICE", ".gitignore")):
                continue
            if path.startswith("packaging/"):
                selected.add("packaging")
            elif path.startswith("client/"):
                selected.update(("unit", "windows"))
            elif path.startswith(("management/", "idp/", "combined/")):
                selected.add("auth")
            else:
                # Общий код и неизвестная область требуют всех быстрых наборов.
                selected.update(CHECKS)
    elif event != "push":
        raise ValueError("Неизвестное событие CI: " + event)
    return {check: check in selected for check in CHECKS}


def main():
    event = os.environ["CI_EVENT"]
    paths = []
    if event == "pull_request":
        paths = subprocess.check_output(
            ["git", "diff", "--name-only", "-z", os.environ["CI_BASE_SHA"],
             os.environ["CI_HEAD_SHA"], "--"], text=True).split("\0")
        paths = [path for path in paths if path]
    result = select_checks(event, os.environ.get("CI_SUITE", ""),
                           os.environ.get("CI_TARGETS", ""), paths)
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as output:
        for check, selected in result.items():
            output.write(f"{check}={str(selected).lower()}\n")
    with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a") as summary:
        summary.write("\n### Выбранные продуктовые проверки\n\n")
        summary.write("`" + json.dumps(result, ensure_ascii=False) + "`\n\n")
        summary.write("Сборки установщиков автоматически не запускаются.\n")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
