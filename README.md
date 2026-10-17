# Aheadscale client

Aheadscale connects a person's devices into a private network. This is the public
client fork of [NetBird v0.80.0](https://github.com/netbirdio/netbird/tree/v0.80.0),
pinned to `fca64287cf51a85552a41b022013abbfdb335452`.

**Operator preview.** CI artifacts are unsigned test installers, not public
production releases. Account self-registration, account isolation, five-device
quotas and automatic private DNS must be verified on the service before public
onboarding. Windows publisher signing, Apple Developer ID/notarization and the
Aheadscale signed update feed are not yet provisioned. Do not publish these
artifacts on a customer-facing CDN or disable OS security settings to install them.

## What changes

- Aheadscale name, artwork and separate app/service/profile/IPC identifiers.
- Configured server: `https://aheadscale.ru:443`; operator-only `internal` builds
  use the temporary Tailscale bootstrap endpoint from the build manifest.
- Browser sign-in followed by automatic connection; no Cloud/Self-hosted selector
  or editable server field in the normal settings flow.
- One desktop package includes the daemon and UI. Linux servers have a CLI-only
  package. Existing NetBird profiles and installations are not adopted.
- Upstream automatic downloads and installation are disabled. A preview must not
  replace itself or a separate NetBird installation with a NetBird release.

## Build

See [`packaging/aheadscale/upstream.json`](packaging/aheadscale/upstream.json) for
pinned tools and source revision. GitHub Actions builds Linux amd64 desktop/CLI,
Linux arm64 CLI, Windows amd64/arm64 and macOS Intel/Apple Silicon previews.

With the upstream platform build prerequisites and pinned Wails generator:

```sh
bash packaging/aheadscale/build.sh linux amd64 internal
```

The output is under `dist/`. Building packages does not install a service on the
builder. Do not run the network client on a server whose connectivity matters.
Windows package preparation additionally needs the pinned Wintun and NSIS plugins
and the Microsoft WebView2 bootstrapper, staged by the workflow. The bootstrapper
can download WebView2 if missing; offline Windows installation is not yet proven.

A channel change is a new service environment, not an account migration. Test
accounts from the private bootstrap must not be silently moved to public service.
Official packages reject saved profiles or MDM management addresses from another
environment without changing the stored keys or account registration.

## Verification and release gates

[Acceptance checklist](docs/aheadscale/acceptance.md) distinguishes compilation,
package inspection and actual installation/network checks on each OS. A green
build does not establish that the full public service is ready. The preview
workflow has read-only repository permissions and no signing or release secrets.
No release/CDN workflow is enabled until the acceptance and signing gates pass.

## Upstream and licensing

Upstream module paths and protocol identifiers remain compatible with NetBird.
The upstream monorepo is preserved; only client and packaging changes are intended
for this distribution. [LICENSE](LICENSE), original copyright notices and
[distribution notices](packaging/aheadscale/NOTICES.md) are retained. The upstream
workflows are archived under `.github/upstream-workflows`; the fork runs only its
own preview pipeline.
