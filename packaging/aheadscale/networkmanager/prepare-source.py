#!/usr/bin/env python3
"""Распаковать только проверенный upstream; применение patch выполняет вызывающий build."""
import hashlib
import json
import os
from pathlib import Path
import sys
import tarfile
import urllib.request

root, work = map(Path, sys.argv[1:])
pin = json.loads((root / 'packaging/aheadscale/networkmanager-source.json').read_text())
archive = Path(os.environ['AHEADSCALE_NETWORKMANAGER_ARCHIVE']) if os.environ.get('AHEADSCALE_NETWORKMANAGER_ARCHIVE') else work / 'source.tar.gz'
if not archive.exists():
    with urllib.request.urlopen(pin['archive_url'], timeout=60) as source:
        archive.write_bytes(source.read())
patch = root / 'packaging/aheadscale/networkmanager/aheadscale.patch'
for file, key in ((archive, 'archive_sha256'), (patch, 'patch_sha256')):
    if hashlib.sha256(file.read_bytes()).hexdigest() != pin[key]:
        raise SystemExit('Хеш исходников NetworkManager не совпадает: ' + key)
raw = work / 'raw'
with tarfile.open(archive) as source:
    source.extractall(raw, filter='data')
children = list(raw.iterdir())
if len(children) != 1 or not children[0].is_dir():
    raise SystemExit('Неожиданная структура архива upstream')
children[0].rename(work / 'source')
