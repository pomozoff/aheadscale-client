#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
out=${1:?output package is required}
plist=$(mktemp "${TMPDIR:-/tmp}/aheadscale-components.XXXXXX")
trap 'rm -f "$plist"' EXIT
pkgbuild --analyze --root payload "$plist"
python3 - "$plist" <<'PY'
from pathlib import Path
import plistlib
import sys

path = Path(sys.argv[1])
components = plistlib.loads(path.read_bytes())
expected = [item for item in components if item.get("RootRelativeBundlePath") == "Applications/Aheadscale.app"]
if len(expected) != 1:
    raise SystemExit("Expected one canonical Aheadscale application component")
expected[0].update(BundleIsRelocatable=False, BundleIsVersionChecked=False,
                   BundleHasStrictIdentifier=True, BundleOverwriteAction="upgrade")
path.write_bytes(plistlib.dumps(components))
PY
pkgbuild --root payload --component-plist "$plist" --scripts scripts --ownership recommended \
    --identifier ru.aheadscale.client.native.pkg --version "$(cat package-version.txt)" \
    --install-location / "$out"
