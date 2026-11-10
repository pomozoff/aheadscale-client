import Foundation
import NetworkExtension

public final class PacketTunnelProvider: NEPacketTunnelProvider {
    private let proofLock = NSLock()
    private var proof: ProviderProofSession?
    private var proofGeneration: UUID?

    public override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        let configuration = (protocolConfiguration as? NETunnelProviderProtocol)?.providerConfiguration
        if #available(macOS 13.0, *),
           configuration?["aheadscale.prototype"] as? String == "session-v1",
           configuration?["aheadscale.bridge-proof"] as? Bool == true {
            let generation = UUID()
            let session = ProviderBridgeProof { [weak self] result in
                if let self {
                    self.proofLock.lock()
                    if self.proofGeneration == generation { self.proof = nil; self.proofGeneration = nil }
                    self.proofLock.unlock()
                }
                let message: String
                switch result {
                case .success:
                    NSLog("Aheadscale provider bridge proof: IPv4/IPv6 PASS; engine not attached")
                    message = "Aheadscale: защищённый канал к root-службе проверен; VPN-движок ещё не подключён."
                case .failure:
                    NSLog("Aheadscale provider bridge proof: refused or cancelled")
                    message = "Aheadscale: проверка защищённого канала не завершена; VPN не запущен."
                }
                completionHandler(NSError(domain: "ru.aheadscale.client.tunnel", code: 2,
                                          userInfo: [NSLocalizedDescriptionKey: message]))
            }
            proofLock.lock()
            guard proof == nil else {
                proofLock.unlock()
                completionHandler(NSError(domain: "ru.aheadscale.client.tunnel", code: 3))
                return
            }
            proof = session
            proofGeneration = generation
            proofLock.unlock()
            session.start()
            return
        }
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
        proofLock.lock()
        let session = proof
        proof = nil
        proofGeneration = nil
        proofLock.unlock()
        session?.stop()
        completionHandler()
    }
}
