# Aheadscale browser registration

Approved 18 October 2026: use the system browser and complete Aheadscale branding.

The existing OAuth/Dex request, state, redirect URI and PKCE challenge are preserved.
Sign up posts email/password to the same-origin /oauth2/signup, creates a local
identity and its independent account/network, then submits the original login
form automatically. The password stays in the page fields for that one request;
no application storage, URL, token API or custom password grant is introduced.

AS_SELF_REGISTRATION=true explicitly enables this prototype mode. It turns off
single-account email-domain grouping, provides the aheadscale.internal private
DNS suffix and enforces five registered devices (offline included). Enrollment
uses the store's global lock and a transaction; the guarantee applies to this
single-process server. Distributed enrollment needs a database-level quota gate
before adding replicas. Existing account/device keys and issuer/client IDs stay
unchanged. Provisioning failure rolls back the local identity and logs failures.

Signup requires the exact configured Origin, JSON, bounded request size, email
validation, upstream-compatible 8–72-byte passwords, a bounded worker count and
rate limiting. This prototype has no mail provider: ownership verification and
self-service password recovery remain public-service prerequisites. Do not
represent this operator test as a finished commercial/public pilot.

The server uses embedded Aheadscale templates, marks and #2563eb. Dashboard
v2.94.0 is forked at https://github.com/pomozoff/aheadscale-dashboard. Both retain
upstream licenses and offer corresponding source. Internal protocol identifiers
and config keys remain compatible; client installers do not require rebuilding.

Build ./combined with Go 1.26.7, CGO_ENABLED=1, gcc and an Ubuntu 24.04-compatible
runtime; package with packaging/aheadscale/Dockerfile.server-runtime. Deploy only
the project's server/dashboard containers, preserving its config and SQLite
volume. Source, image IDs and browser verification are recorded in the platform
runbook after deployment. Browser plugin was unavailable; regular Playwright was
used against an isolated fixture with synthetic data and loopback ports.

Validation on 18 October 2026: real SQLite/embedded-IdP tests cover distinct
accounts for equal email domains, repeat signup, malformed/cross-origin requests
and concurrent fifth-device enrollment (RED to GREEN). Existing IdP/Dex/server
configuration tests pass. The isolated real-browser run registered two users,
checked separate account/user API views, repeated CLI PKCE login and signed into
the branded dashboard. Desktop/mobile screenshots and #2563eb were checked.
The owner explicitly authorized publishing these server changes in the existing
public fork and updating the live service after the automatic publication gate.
