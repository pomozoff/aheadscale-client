#!/bin/sh
set -eu
tool=${0##*/}
case "$tool" in rpm|rpmbuild) ;; *) echo 'Use a symlink named rpm or rpmbuild' >&2; exit 1 ;; esac
case "${AHEADSCALE_RPM_ARCH:?}:${AHEADSCALE_RPM_CHANNEL:?}" in
  amd64:internal|amd64:public|arm64:internal|arm64:public) ;;
  *) echo 'Invalid RPM build target' >&2; exit 1 ;;
esac
binaries="$PWD/build/linux-$AHEADSCALE_RPM_ARCH-$AHEADSCALE_RPM_CHANNEL"
test -d "$binaries" && test -d "$PWD/dist"
# Только собственные бинарники/сборочные каталоги; исходники и credentials не монтируются.
exec docker run --rm --name "aheadscale-rpm-$tool-$$" --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges --pids-limit 128 --memory 2g --cpus 2 \
  --user "$(id -u):$(id -g)" --tmpfs /tmp:rw,nosuid,nodev --tmpfs /var/tmp:rw,nosuid,nodev \
  --env HOME=/tmp --env SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-0}" \
  --mount "type=bind,src=$binaries,dst=$binaries" \
  --mount "type=bind,src=$PWD/dist,dst=$PWD/dist,readonly" \
  "${AHEADSCALE_RPM_IMAGE:?}" "$tool" "$@"
