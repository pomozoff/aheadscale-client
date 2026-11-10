import Foundation
import NetworkExtension

public final class PacketTunnelProvider: NEPacketTunnelProvider {
    private let lifecycle = TunnelLifecycle()

    public override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let configuration = (protocolConfiguration as? NETunnelProviderProtocol)?.providerConfiguration
        if #available(macOS 13.0, *),
           configuration?["aheadscale.prototype"] as? String == "session-v1",
           configuration?["aheadscale.bridge-proof"] as? Bool == true {
            lifecycle.start(makeProof: { ProviderBridgeProof(completion: $0) }, completion: completionHandler)
        } else {
            lifecycle.start(completion: completionHandler)
        }
    }

    public override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        lifecycle.stop(completion: completionHandler)
    }
}
