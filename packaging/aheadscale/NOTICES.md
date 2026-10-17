# Aheadscale notices

Aheadscale is a downstream distribution of NetBird v0.80.0, commit
fca64287cf51a85552a41b022013abbfdb335452. Original copyright notices and
license texts are retained in the upstream source and the bundled LICENSE.

Client code is distributed under the upstream BSD-3-Clause terms. The upstream
monorepo also contains server components under AGPLv3; the Aheadscale client
pipeline does not build or ship those servers. Dependencies retain their own
licenses. NetBird contributors do not endorse Aheadscale.

Windows packages include the original signed Wintun 0.14.1 driver; its license
from the downloaded archive is bundled with the driver. Native preview packages
are not signed with an Aheadscale publisher certificate or notarized by Apple.
They are for operator testing and must not be published as production installers.
