#!/bin/sh
# Сборка policy в собственной копии Fedora store; политика хоста не меняется.
set -eu
project=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
output=${1:?Нужен выходной путь nm_aheadscale.pp}
mkdir -p "$(dirname -- "$output")"
output=$(CDPATH= cd -- "$(dirname -- "$output")" && pwd)/$(basename -- "$output")
workspace=$(mktemp -d "${TMPDIR:-/tmp}/aheadscale-nm-policy.XXXXXX")
trap 'rm -rf -- "$workspace"' EXIT HUP INT TERM
python3 "$project/packaging/aheadscale/networkmanager/prepare-source.py" "$project" "$workspace"
cd "$workspace/source"
git apply "$project/packaging/aheadscale/networkmanager/aheadscale.patch"
image=${AHEADSCALE_POLICY_BUILDER_IMAGE:-local/aheadscale-nm-fedora44:20261028}
if ! docker image inspect "$image" >/dev/null 2>&1; then
    docker build -t "$image" -f "$project/packaging/aheadscale/networkmanager/Dockerfile.fedora" "$project/packaging/aheadscale/networkmanager"
fi
docker run --rm --network=none --cpus=2 --memory=2g -e OUT_UID="$(id -u)" -e OUT_GID="$(id -g)" \
    -v "$workspace/source/packaging/selinux:/policy" -w /policy "$image" sh -ec '
    store=$(mktemp -d)
    trap '\''rm -rf "$store"; chown -R "$OUT_UID:$OUT_GID" /policy'\'' EXIT
    make
    cp -a /var/lib/selinux/targeted "$store/"
    semodule -s targeted -S "$store" -N -i nm_aheadscale.pp
    '
cp packaging/selinux/nm_aheadscale.pp "$output"
