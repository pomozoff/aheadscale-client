# Aheadscale acceptance

Do not mark an OS installation or network scenario passed from compilation alone.

| Check | Linux amd64 | Windows amd64 | macOS arm64 |
| --- | --- | --- | --- |
| Configuration/IPC unit tests | See CI | Cross-build only | Build only |
| CLI and desktop compilation | CI | CI | CI |
| Native installer contents and syntax | CI | NSIS compile | pkgbuild/payload listing |
| Windows adapter identity on native runner | N/A | Passed in CI | N/A |
| Service package installation/removal on CI runner | Passed | Pending | Pending |
| Clean-machine install, normal-user UI, system permission flow | Pending | Pending | Pending |
| Browser sign-in and automatic connection | Pending | Pending | Pending |
| Reboot/reconnect retains registration and account | Pending | Pending | Pending |
| All own devices including offline, search/copy name/IP | Pending | Pending | Pending |
| Private DNS and useful SSH/HTTP connection | Pending | Pending | Pending |
| In-place upgrade preserves keys and device identity | Pending | Pending | Pending |
| Uninstall preserves other NetBird/Tailscale installations | Pending | Pending | Pending |
| Publisher signature/notarization | Repository signature pending | Pending certificate | Pending Developer ID/notarization |

CI evidence: [preview run 39022500060](https://github.com/pomozoff/aheadscale-client/actions/runs/39022500060), source `b9f43a05d0cd8d4f71b5650fc53037ad6f539e7f`. The three selected platform jobs passed and their artifacts remain available; the overall run was canceled for macOS Intel, which remains deferred. Windows/Linux arm64 are added in a separate selected run.

## Public service gates

- Server signup with verified email/password and idempotent creation of an isolated
  personal account/network. No joining another account by matching email domain.
- Atomic server-side five-registered-device limit; removing a device frees a slot.
- Cross-account denial for REST, management, peer directory, DNS and traffic.
- Names and addresses distributed automatically on all supported OSes, including
  Windows/macOS DNS behavior; offline entries do not imply reachability.
- Own signed updates and CDN provenance. No fallback to upstream packages/keys.
- Real OS checks above, publisher signing and macOS notarization.

Until all gates are recorded with evidence, artifacts are operator previews only.

## Preview installer behavior

Linux requires a systemd-based Ubuntu-compatible Debian environment. Desktop
packages for amd64 and arm64 require GTK 4.14+ and WebKitGTK 6 (Ubuntu 24.04
or a compatible newer system). Separate CLI packages remain available. Windows
amd64 requires Windows 10 or newer; Windows 7 is unsupported. For arm64 VM
testing on Apple Silicon use Windows 11 arm64. Windows packages preserve upstream WebView2 detection/bootstrapper behavior, which can
require access to Microsoft on a machine without that runtime. Mac packages are
ad-hoc signed for testing and are not Developer ID signed or notarized.

macOS previews include `/Applications/Uninstall Aheadscale.command`: it requests
admin permission, stops/removes only Aheadscale, and keeps registration keys and
preferences. Linux package removal also keeps registration. Windows retains data
unless the user explicitly selects its Aheadscale-only deletion option.

## Resource separation

Aheadscale uses its own service, configuration/state directories, UI preferences,
IPC endpoints, `as0`/`utun120` interface, UDP 51830 default, firewall chains/table,
routing table 7136, connection-tracking zone and packet-mark range. Existing
NetBird/Tailscale files are not migrated. Concurrent overlapping VPN routes and
DNS still require the real OS coexistence checks above.

Preview diagnostics are generated locally; upstream support upload and update
workers are disabled. The included BSD/third-party notices retain attribution.

CLI flag environment settings use `AS_` (for example `AS_CONFIG`), and state
uses `AS_STATE_DIR`. Legacy `NB_CONFIG`/`WT_CONFIG` variables cannot select a
NetBird/Wiretrustee profile for Aheadscale.

## Operator integration run (before any public release)

Use disposable clean machines with a snapshot for each supported OS. Use the
internal installers only against the private bootstrap, and public-endpoint
installers only against the public test service. Never switch channels on a
registered machine or reuse its account/device keys as a migration strategy.

1. Install as a normal user with the OS elevation prompt. Open Aheadscale, use
   its single sign-in button and complete browser authentication. Record that no
   server URL was requested and the connection completes without returning to
   the app. Repeat for a new email/password signup once the server supports it.
2. Join Linux, Windows and macOS devices to account A. Compare their directories
   with the server's account-A devices: name, private IP and offline entries
   must agree. Search by name and IP, copy both values, stop one device and
   verify its offline entry remains on the other two.
3. For each OS, resolve the copied private FQDN with its application resolver:
   Linux `getent ahostsv4`, macOS `dscacheutil -q host -a name`, Windows
   `[System.Net.Dns]::GetHostAddresses()`. Compare with the directory IP, then
   make a real SSH/HTTP connection to the other device. Repeat using its short
   name, without a hosts-file edit or manual DNS configuration. Record the
   actual private suffix supplied by the server rather than assuming one.
4. Reboot each machine, change Wi-Fi/network and restart the app. Check automatic
   reconnection, the same server device ID/key, and a useful connection by name.
   Install a later Aheadscale preview over it and repeat; a successful compiler
   or reinstall of the same version does not prove upgrade preservation.
5. With five registered account-A devices (including offline), attempt a sixth.
   It must be rejected by the server. Delete one, enroll another and check the
   freed slot. Race two enrollments for the last slot: exactly one may succeed.
6. Join a device to account B, including a user with the same email domain as A.
   Check that neither directory/DNS/API nor SSH/HTTP traffic crosses accounts.
   A UI-only filter is insufficient evidence of server isolation.
7. Repeat install/upgrade/remove with real NetBird and Tailscale already present.
   Check their services, registrations and useful connections before and after.
   Removing Aheadscale must stop only its service and remove its binaries/OS
   entries; retained Aheadscale keys must reconnect after reinstall. On Windows
   separately test the explicit deletion-of-Aheadscale-data option.
8. Record OS/build versions, source commit, installer SHA256, account/device test
   IDs, outcomes and blockers. Keep credentials, private keys and raw diagnostic
   bundles out of public evidence. Verify final publisher signatures on the
   downloaded CDN bytes before changing any Pending status to Passed.

These steps are not yet executed and are not replaced by the CI installation
check. The server registration/isolation/quota gates and signing credentials
are prerequisites for the complete run.
