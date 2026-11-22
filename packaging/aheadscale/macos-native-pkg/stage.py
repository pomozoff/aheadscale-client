"""Stage the ARM64 package without signing keys or host installation."""
import argparse
import plistlib
import shutil
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("inputs", type=Path)
parser.add_argument("version")
args = parser.parse_args()
root = Path(__file__).resolve().parents[3]
pkg = root / "packaging/aheadscale"
native = pkg / "macos-network-extension"
this = Path(__file__).resolve().parent
payload = args.inputs / "payload"
scripts = args.inputs / "scripts"
app = payload / "Applications/Aheadscale.app/Contents"
provider = app / "Library/SystemExtensions/ru.aheadscale.client.tunnel.systemextension/Contents"
gui = app / "Library/LoginItems/Aheadscale UI.app/Contents"
bridge = app / "Library/LaunchServices/AheadscaleNativeEngineBridge"
engine = payload / "Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine"


def copy(src, dst, executable=False):
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(src, dst)
    dst.chmod(0o755 if executable else 0o644)


def write_plist(dst, value):
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_bytes(plistlib.dumps(value))


for name, destination in [
    ("AheadscaleExtensionSetup", app / "MacOS/AheadscaleExtensionSetup"),
    ("aheadscale-native-cli", app / "MacOS/aheadscale"),
    ("aheadscale-native-ui", gui / "MacOS/aheadscale-ui"),
    ("AheadscaleNativeEngineBridge", bridge),
    ("aheadscale-native", engine),
    ("AheadscaleTunnelExtension", provider / "MacOS/AheadscaleTunnelExtension"),
]:
    copy(args.inputs / "bin" / name, destination, executable=True)

short_version, build = args.version.split("-aheadscale.", 1)
if not build.isdecimal() or not all(part.isdecimal() for part in short_version.split(".")):
    raise SystemExit("Version must be a numeric release and Aheadscale build")
app_info = plistlib.loads((native / "app-Info.plist").read_bytes())
app_info.update(
    CFBundleName="Aheadscale",
    CFBundleDisplayName="Aheadscale",
    CFBundleShortVersionString=short_version,
    CFBundleVersion=build,
    CFBundleIconFile="icons",
    AheadscaleInstalled=True,
)
write_plist(app / "Info.plist", app_info)
write_plist(gui / "Info.plist", {
    "CFBundleIdentifier": "ru.aheadscale.client.ui",
    "CFBundleExecutable": "aheadscale-ui",
    "CFBundleName": "Aheadscale",
    "CFBundleDisplayName": "Aheadscale",
    "CFBundlePackageType": "APPL",
    "CFBundleVersion": build,
    "CFBundleShortVersionString": short_version,
    "CFBundleIconFile": "icons",
    "LSUIElement": True,
    "LSMinimumSystemVersion": "13.0",
    "NSHighResolutionCapable": True,
})
copy(native / "Info.plist", provider / "Info.plist")
label = "Y24464MUL3.ru.aheadscale.client.bridge"
copy(this / "launchd.plist", payload / f"Library/LaunchDaemons/{label}.plist")
copy(this / "uninstall.command", payload / "Applications/Uninstall Aheadscale.command", executable=True)
for relative in (app, gui):
    copy(pkg / "icon.icns", relative / "Resources/icons.icns")
for name, source in [("LICENSE", root / "LICENSE"), ("NOTICES.md", pkg / "NOTICES.md"),
                     ("macos-gui-process.js", pkg / "scripts/macos-gui-process.js")]:
    copy(source, app / "Resources" / name)
for name in ("preinstall", "postinstall", "installer-common.sh"):
    copy(this / name, scripts / name, executable=True)
copy(pkg / "scripts/macos-gui-process.js", scripts / "macos-gui-process.js")
for name in ("app", "provider"):
    copy(native / f"{name}.entitlements", args.inputs / f"{name}.entitlements")
copy(this / "bridge.entitlements", args.inputs / "bridge.entitlements")
for name in ("package.sh", "sign-on-macbook.sh"):
    copy(this / name, args.inputs / name, executable=True)
(args.inputs / "package-version.txt").write_text(f"{short_version}.{build}\n")
