#!/usr/bin/env python3
"""Собрать один закрытый preview EXE из проверенных native и стандартных сборок."""
import argparse
import base64
import hashlib
import json
import re
from pathlib import Path
import shutil
import subprocess
import tempfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest().upper()


def loader(hashes, operation):
    # Блокируем изменение/подмену файлов до конца выполнения: проверка SHA
    # и последующее чтение выполняются при удержании тех же открытых файлов.
    table = ";".join("'" + name + "'='" + sha + "'" for name, sha in hashes.items())
    script = r"""
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';$locks=@();$artifactRoot=$env:AHEADSCALE_SETUP_ROOT
try {
 $expected=@{TABLE}
 foreach($name in $expected.Keys){
  $f=[IO.File]::Open((Join-Path $artifactRoot $name),[IO.FileMode]::Open,[IO.FileAccess]::Read,[IO.FileShare]::Read)
  $locks+=$f;$sha=[Security.Cryptography.SHA256]::Create()
  try{$actual=[BitConverter]::ToString($sha.ComputeHash($f)).Replace('-','')}finally{$sha.Dispose()}
  if($actual -ne $expected[$name]){throw 'Повреждены файлы установщика Aheadscale'}
 }
 $operation='OPERATION'
 & ([scriptblock]::Create([IO.File]::ReadAllText((Join-Path $artifactRoot 'desktop-install.ps1'))))
 exit 0
} catch { [Console]::Error.WriteLine($_.Exception.Message+"`n"+$_.ScriptStackTrace);exit 1 }
finally {foreach($f in $locks){$f.Dispose()}}
""".replace("TABLE", table).replace("OPERATION", operation)
    return base64.b64encode(script.encode("utf-16le")).decode()


def bootstrap(name, sha):
    # Stock NSIS ограничивает команду 1024 символами. Короткий bootstrap
    # проверяет SHA доверенного loader и удерживает его без разрешения записи.
    script = "$f=[IO.File]::OpenRead($env:AHEADSCALE_SETUP_ROOT+'\\" + name + "');try{if((Get-FileHash $f.Name).Hash-ne'" + sha + "'){exit 1};&([scriptblock]::Create([IO.File]::ReadAllText($f.Name)))}finally{$f.Dispose()}"
    encoded = base64.b64encode(script.encode("utf-16le")).decode()
    if len(encoded) + 160 >= 1024:
        raise ValueError("Bootstrap превышает ограничение NSIS")
    return encoded


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--native", type=Path, required=True)
    parser.add_argument("--standard-installer", type=Path, required=True)
    parser.add_argument("--standard-cli", type=Path, required=True)
    parser.add_argument("--standard-ui", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    manifest = json.loads((args.native / "build-manifest.json").read_text(encoding="utf-8-sig"))
    if manifest["architecture"] != "amd64":
        raise SystemExit("Первый единый preview проверяется только на Windows x64")
    for field, sha in (("packageFile", "sha256"), ("nativeUIFile", "nativeUISha256")):
        name = manifest[field]
        if Path(name).name != name or digest(args.native / name) != manifest[sha].upper():
            raise SystemExit("Не совпала контрольная сумма native-компонента")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="aheadscale-desktop-") as folder:
        work = Path(folder)
        payload = work / "payload"
        payload.mkdir()
        for name in (manifest["packageFile"], manifest["nativeUIFile"], "Aheadscale.VpnPrototype.cer", "build-manifest.json"):
            shutil.copy2(args.native / name, payload / name)
        for name in ("install-test.ps1", "remove-test.ps1", "desktop-install.ps1"):
            shutil.copy2(repo / "packaging/windows-vpn-prototype/scripts" / name, payload / name)
        shutil.copy2(repo / "client/internal/windowsstartup/task.ps1", payload / "startup-task.ps1")
        shutil.copy2(args.standard_installer, payload / "standard-setup.exe")
        shutil.copy2(repo / "LICENSE", payload / "LICENSE")
        shutil.copy2(repo / "packaging/aheadscale/NOTICES.md", payload / "NOTICES.md")
        (payload / "desktop-manifest.json").write_text(json.dumps({
            "standardSha256": digest(args.standard_installer),
            "standardCliSha256": digest(args.standard_cli),
            "standardUiSha256": digest(args.standard_ui),
        }), encoding="utf-8")
        # Не принимать ошибочно выбранный или устаревший стандартный EXE.
        extracted = work / "standard"
        subprocess.run(["7z", "x", "-y", "-o" + str(extracted), str(args.standard_installer)], check=True, stdout=subprocess.DEVNULL)
        for name, original in (("aheadscale.exe", args.standard_cli), ("aheadscale-ui.exe", args.standard_ui)):
            matches = [p for p in extracted.rglob("*") if p.name.lower() == name]
            if len(matches) != 1 or digest(matches[0]) != digest(original):
                raise SystemExit("Стандартный установщик содержит другие бинарники: " + name)
        hashes = {p.name: digest(p) for p in payload.iterdir()}
        for operation, name in (("install", "i.ps1"), ("remove", "r.ps1")):
            (payload / name).write_text(base64.b64decode(loader(hashes, operation)).decode("utf-16le"), encoding="utf-8-sig")
        install = bootstrap("i.ps1", digest(payload / "i.ps1"))
        remove = bootstrap("r.ps1", digest(payload / "r.ps1"))
        version = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)-aheadscale\.(\d+)", manifest["clientVersion"])
        if not version or any(int(part) > 65535 for part in version.groups()):
            raise SystemExit("Неверная версия Windows")
        pe_version = ".".join(version.groups())
        output = args.output.resolve()
        # Все подставляемые пути принадлежат сборщику. Символ $ в NSIS экранируется.
        def quote(path):
            return str(path).replace("$", "$$").replace('"', '$\\"')
        source = r'''
!include "MUI2.nsh"
!include "x64.nsh"
Name "Aheadscale Preview"
OutFile "OUTPUT"
InstallDir "$PROGRAMFILES64\Aheadscale\native"
RequestExecutionLevel admin
Unicode true
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUninstDetails show
!define MUI_ICON "APP_ICON_PATH"
!define MUI_UNICON "APP_ICON_PATH"
VIProductVersion PE_VERSION_NUMBER
VIAddVersionKey /LANG=1033 "ProductName" "Aheadscale"
VIAddVersionKey /LANG=1033 "CompanyName" "Aheadscale"
VIAddVersionKey /LANG=1033 "LegalCopyright" "NetBird Authors and Aheadscale contributors"
VIAddVersionKey /LANG=1033 "FileDescription" "Aheadscale Preview"
VIAddVersionKey /LANG=1033 "FileVersion" "PE_VERSION_NUMBER"
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Открыть Aheadscale"
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchClient
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "PAYLOAD/LICENSE"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Russian"
Section
 InitPluginsDir
 SetOutPath "$PLUGINSDIR\payload"
 File /r "PAYLOAD/*"
 System::Call 'kernel32::SetEnvironmentVariableW(w "AHEADSCALE_SETUP_ROOT", w "$PLUGINSDIR\payload") i.r0'
 nsExec::ExecToStack '"$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -EncodedCommand INSTALL'
 Pop $0
 Pop $1
 DetailPrint "$1"
 StrCmp $0 0 success
 MessageBox MB_OK|MB_ICONSTOP "Установка Aheadscale не завершена: $1"
 SetErrorLevel 1
 Abort
 success:
 SetOutPath "$INSTDIR"
 CopyFiles /SILENT "$PLUGINSDIR\payload\*" "$INSTDIR"
 WriteUninstaller "$INSTDIR\uninstall.exe"
 SetRegView 64
 WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Aheadscale" "UninstallString" '"$INSTDIR\uninstall.exe"'
SectionEnd
Function LaunchClient
 ShellExecAsUser::ShellExecAsUser "" "$INSTDIR\aheadscale-ui.exe"
FunctionEnd
Section Uninstall
 System::Call 'kernel32::SetEnvironmentVariableW(w "AHEADSCALE_SETUP_ROOT", w "$INSTDIR") i.r0'
 nsExec::ExecToStack '"$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -EncodedCommand REMOVE'
 Pop $0
 Pop $1
 DetailPrint "$1"
 StrCmp $0 0 restored
 MessageBox MB_OK|MB_ICONSTOP "Не удалось безопасно удалить Aheadscale: $1. Компоненты сохранены для восстановления."
 SetErrorLevel 1
 Abort
 restored:
 SetRegView 64
 ReadRegStr $1 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Aheadscale"
 StrCmp $1 '"$PROGRAMFILES64\Aheadscale\aheadscale-ui.exe"' clearRun
 StrCmp $1 '"$INSTDIR\aheadscale-ui.exe"' clearRun keepRun
 clearRun:
 DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Aheadscale"
 keepRun:
 ExecWait '"$PROGRAMFILES64\Aheadscale\aheadscale_uninstall.exe" /S' $0
 SetErrorLevel $0
SectionEnd
'''.replace("APP_ICON_PATH", quote(repo / "client/ui/build/windows/icon.ico")).replace("PE_VERSION_NUMBER", pe_version).replace("OUTPUT", quote(output)).replace("PAYLOAD", quote(payload)).replace("INSTALL", install).replace("REMOVE", remove)
        script = work / "desktop.nsi"
        script.write_text(source, encoding="utf-8")
        subprocess.run(["makensis", "-V2", str(script)], check=True)
    print(str(args.output), digest(args.output))


if __name__ == "__main__":
    main()
