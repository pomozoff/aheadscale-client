import Foundation
import NetworkExtension
import Darwin
import AheadscaleBridgeProbe

public final class PacketTunnelProvider: NEPacketTunnelProvider {
    private let lifecycle = TunnelLifecycle()
    private let nativeLock = NSLock()
    private var nativeSession: NativeEngineSession?

    public override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let configuration = (protocolConfiguration as? NETunnelProviderProtocol)?.providerConfiguration
        if #available(macOS 13.0, *),
           configuration?["aheadscale.prototype"] as? String == "session-v1",
           configuration?["aheadscale.native-engine"] as? Bool == true {
            nativeLock.lock()
            guard nativeSession == nil else {
                nativeLock.unlock()
                completionHandler(NSError(domain: "ru.aheadscale.client.tunnel", code: 6))
                return
            }
            let session = NativeEngineSession(flow: packetFlow, remote: NativeSessionClient(), applySettings: { [weak self] config, reply in
                guard let self else { reply(nil, NSError(domain: "ru.aheadscale.client.tunnel", code: 4)); return }
                do {
                    let settings = try config.networkSettings()
                    self.setTunnelNetworkSettings(settings) { error in
                        if let error { reply(nil,error); return }
                        reply(Self.interfaceName(address: String(config.address.split(separator: "/")[0])),nil)
                    }
                } catch { reply(nil,error) }
            }, completion: completionHandler, onFailure: { [weak self] error in self?.cancelTunnelWithError(error) })
            nativeSession = session
            nativeLock.unlock()
            session.start()
        } else if #available(macOS 13.0, *),
           configuration?["aheadscale.prototype"] as? String == "session-v1",
           configuration?["aheadscale.bridge-proof"] as? Bool == true {
            lifecycle.start(makeProof: { ProviderBridgeProof(completion: $0) }, completion: completionHandler)
        } else {
            lifecycle.start(completion: completionHandler)
        }
    }

    public override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        nativeLock.lock()
        let session = nativeSession
        nativeSession = nil
        nativeLock.unlock()
        if let session { session.stop(completion: completionHandler) }
        else { lifecycle.stop(completion: completionHandler) }
    }

    // Public getifaddrs, never private KVC/utun descriptor introspection. The
    // root engine independently checks that this utun owns the assigned address.
    private static func interfaceName(address: String) -> String? {
        var list: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&list) == 0 else { return nil }
        defer { freeifaddrs(list) }
        var item = list
        var matches = Set<String>()
        while let entry = item {
            defer { item = entry.pointee.ifa_next }
            guard let addr = entry.pointee.ifa_addr, addr.pointee.sa_family == UInt8(AF_INET) else { continue }
            var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            guard getnameinfo(addr, socklen_t(addr.pointee.sa_len), &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0,
                  String(cString: host) == address else { continue }
            let name = String(cString: entry.pointee.ifa_name)
            if name.hasPrefix("utun") { matches.insert(name) }
        }
        return matches.count == 1 ? matches.first : nil
    }
}
