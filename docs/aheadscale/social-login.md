# Russian social login

Requested 18 October 2026: VK ID, Yandex ID, Mail.ru and Odnoklassniki.
VK ID supports all three VK/Mail/OK options through one application. Yandex
requires a separate OAuth application. No installer changes are needed.

## Application registration

1. Create an Aheadscale **web** application in [VK ID](https://id.vk.ru/about/business/).
   Website: `https://aheadscale.ru`. Trusted redirect URI:
   `https://aheadscale.ru/oauth2/callback`. Enable the email permission and VK,
   Mail and OK login options offered by the provider. Copy the application ID.
   The PKCE flow does not require the legacy VK application secret.
2. Create an application **for user authorization** in
   [Yandex OAuth](https://oauth.yandex.ru/client/new/id/). Choose web services,
   use the same website/redirect URI and request **email** (`login:email`).
   Copy ClientID and Client secret. Do not request phone/address/birthday access.
3. Store the two IDs and Yandex secret in private operator configuration; never
   send secrets in chat or commit them. Provider approvals/verification and an
   actual user login are necessary before calling the integration enabled.

Registration pages can change; follow the providers' current instructions:
[VK SDK](https://github.com/VKCOM/vkid-web-sdk),
[Yandex registration](https://yandex.ru/dev/id/doc/ru/register-auth).

## Server configuration

`packaging/aheadscale/social-connectors.example.json` is a **partial merge
example**, not a replacement for the existing server configuration. Preserve
all issuer/session/owner/SQLite/relay settings. Put its `connectors` array under
`server.auth` in the private combined-server configuration. The running server
must receive `AS_VKID_CLIENT_ID`, `AS_YANDEX_CLIENT_ID` and
`AS_YANDEX_CLIENT_SECRET` as private environment variables. The `$NAME` references
are resolved at connector opening; missing credentials fail closed. An operator
may instead write literal credentials in the private configuration through the
existing secret-management process. Restart only the Aheadscale server.

With no connector configuration, the existing email Sign in/Sign up remains.
Configured connectors appear in the existing branded method picker; choosing
email opens the same validated form. There are no decorative non-working social
buttons. Static connectors persist in Dex: removing an entry from the file does
not delete its stored connector; use the existing administrator connector deletion
API when disabling a configured provider.

## Identity and session behavior

Dex keeps the original application OAuth request, redirect, PKCE and account
identity. New upstream connectors use authorization codes with state and PKCE;
VK also binds device_id and checks the token response state. Tokens and secrets
are exchanged by the server and never put in browser storage or request URLs.
Provider requests have timeouts, bounded responses, redirect rejection and
redacted errors. Refresh tokens remain in Dex connector session data; refresh
rechecks the provider subject and rejects a change of person. A missing email
permission produces an error rather than fabricating an address. No email
verification claim is invented. Accounts are identified by provider + immutable
user ID; equal email addresses do not link accounts, including local passwords.

## Validation and remaining operator work

Native tests cover all four code/PKCE/profile/refresh paths with synthetic HTTP
providers, invalid state, rejected/missing identity/email, token/profile mismatch,
wrong Yandex application, changed subjects and five distinct accounts sharing
one email. Config tests verify connector forwarding and unchanged defaults.
Actual upstream login, consent, provider approvals and refresh must still be
checked with the owner's registered applications. Until those credentials are
configured, social login remains disabled on the live site.
