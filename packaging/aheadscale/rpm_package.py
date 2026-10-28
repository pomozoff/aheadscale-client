"""Нативный RPM из готовых ELF: зависимости, systemd и сохранение регистрации."""
import os
import re
import shutil
import subprocess
import tarfile
import json
from networkmanager_package import PAYLOAD, prepare_hook, normalize_directories


def package_rpm(root, binaries, staging, output, version, arch, cli_only):
    match = re.fullmatch(r"(\d+\.\d+\.\d+)-aheadscale\.(\d+)", version)
    if not match:
        raise SystemExit("Для RPM нужна версия вида 0.80.0-aheadscale.7")
    target = {"amd64": "x86_64", "arm64": "aarch64"}[arch]
    package_name = "aheadscale-cli" if cli_only else "aheadscale"
    conflict = "aheadscale" if cli_only else "aheadscale-cli"
    work = binaries / ("rpmbuild-cli" if cli_only else "rpmbuild-desktop")
    if work.exists():
        raise SystemExit("Каталог rpmbuild уже существует; нужен новый каталог сборки")
    for directory in ("SOURCES", "SPECS", "BUILD", "BUILDROOT", "RPMS", "SRPMS"):
        (work / directory).mkdir(parents=True)
    unit = staging / "usr/lib/systemd/system/aheadscale.service"
    unit.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(root / "packaging/aheadscale/aheadscale.service", unit)
    unit.chmod(0o644)

    def metadata(info):
        info.uid = info.gid = 0
        info.uname = info.gname = "root"
        info.mtime = int(os.environ.get("SOURCE_DATE_EPOCH", "0"))
        return info

    normalize_directories(staging)
    with tarfile.open(work / "SOURCES/payload.tar.gz", "w:gz") as archive:
        archive.add(staging / "usr", arcname="usr", filter=metadata)
    files = ["%{_bindir}/aheadscale", "%{_unitdir}/aheadscale.service",
             "%dir %{_prefix}/lib/aheadscale", "%{_prefix}/lib/aheadscale/cleanup-networkmanager",
             "%dir %{_docdir}/aheadscale", "%license %{_docdir}/aheadscale/LICENSE", "%doc %{_docdir}/aheadscale/NOTICES.md"]
    if not cli_only:
        files += ["%{_bindir}/aheadscale-ui", "%{_datadir}/applications/org.wails.aheadscale.desktop",
                  "%{_datadir}/icons/hicolor/512x512/apps/aheadscale.png"]
    files += ["%{_prefix}/lib/aheadscale/networkmanager-lifecycle", "%dir %{_datadir}/aheadscale", "%{_datadir}/aheadscale/networkmanager-files.json"]
    if not cli_only:
        files += [*PAYLOAD.values(), "/usr/share/selinux/packages/nm_aheadscale.pp", "%dir /usr/lib/systemd/system/aheadscale.service.d", "%dir /usr/share/doc/aheadscale/networkmanager", "/usr/share/doc/aheadscale/networkmanager/LICENSE", "/usr/share/doc/aheadscale/networkmanager/NOTICES.md", "/usr/share/doc/aheadscale/networkmanager/SOURCE.json"]
    manifest=json.loads((staging / 'usr/share/aheadscale/networkmanager-files.json').read_text())
    replacements = {
        "@NAME@": package_name, "@VERSION@": match[1], "@RELEASE@": "0.aheadscale." + match[2] + ".1",
        "@CONFLICT@": conflict,
        "@GUI_REQUIRES@": "" if cli_only else "Requires: gtk4 >= 4.14\nRequires: webkitgtk6.0\nRequires: NetworkManager\nRequires: NetworkManager-libnm\nRequires: systemd-resolved\nRequires: libnma\nRequires: libnma-gtk4\nRequires: gtk3\nRequires: policycoreutils\nRequires(pre): policycoreutils",
        "@NM_PREPARE@": prepare_hook(root,manifest).replace("%", "%%"),
        "@NM_APPLY@": "/usr/lib/aheadscale/networkmanager-lifecycle apply " + manifest['variant'] + " rpm",
        "@FILES@": "\n".join(files),
        "@RPM_LIFECYCLE@": (root / "packaging/aheadscale/scripts/rpm-lifecycle").read_text().replace("%", "%%"),
    }
    spec = (root / "packaging/aheadscale/aheadscale.spec.in").read_text()
    for token, value in replacements.items():
        spec = spec.replace(token, value)
    path = work / "SPECS/aheadscale.spec"
    path.write_text(spec)
    subprocess.run([os.environ.get("RPMBUILD", "rpmbuild"), "-bb", "--target", target,
                    "--define", "_topdir " + str(work), str(path)], check=True)
    packages = list((work / "RPMS" / target).glob("*.rpm"))
    if len(packages) != 1:
        raise SystemExit("Ожидался ровно один RPM текущего варианта")
    shutil.copy2(packages[0], output)
    subprocess.run([os.environ.get("RPM", "rpm"), "-qip", str(output)], check=True)
