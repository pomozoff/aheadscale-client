import Foundation
import NetworkExtension

public final class PacketTunnelProvider: NEPacketTunnelProvider {
    public override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        // Until the authenticated bridge exists, no OS routes may be installed.
        completionHandler(NSError(
            domain: "ru.aheadscale.client.tunnel",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey:
                "Aheadscale: канал к VPN-службе ещё не подключён. Это тестовое расширение."]
        ))
    }

    public override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        completionHandler()
    }
}
