import Foundation
import Network
import NetworkExtension

enum NativeSessionConfigurationError: Error { case invalid }

// Wire contract with the authenticated root engine; never receives keys or an
// arbitrary route list. Version 1 adds only the assigned IPv4 overlay subnet.
struct NativeSessionConfiguration: Decodable {
    let version: Int
    let generation: String
    let address: String
    let mtu: Int
    let dnsServers: [String]
    let dnsDomains: [String]

    static func parse(_ data: Data) throws -> Self {
        guard !data.isEmpty, data.count <= 4096,
              let object = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              Set(object.keys) == Set(["version", "generation", "address", "mtu", "dnsServers", "dnsDomains"]) else {
            throw NativeSessionConfigurationError.invalid
        }
        let config = try JSONDecoder().decode(Self.self, from: data)
        _ = try config.validatedAddress()
        return config
    }

    func localIPv4Bytes() throws -> Data {
        let (ip, _) = try validatedAddress()
        return Data([UInt8(ip >> 24), UInt8(truncatingIfNeeded: ip >> 16),
                     UInt8(truncatingIfNeeded: ip >> 8), UInt8(truncatingIfNeeded: ip)])
    }

    func networkSettings() throws -> NEPacketTunnelNetworkSettings {
        let (ip, prefix) = try validatedAddress()
        let mask = UInt32.max << (32 - prefix)
        let maskString = Self.text(mask)
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        let ipv4 = NEIPv4Settings(addresses: [Self.text(ip)], subnetMasks: [maskString])
        ipv4.includedRoutes = [NEIPv4Route(destinationAddress: Self.text(ip & mask), subnetMask: maskString)]
        settings.ipv4Settings = ipv4
        let dns = NEDNSSettings(servers: dnsServers)
        dns.matchDomains = dnsDomains
        dns.matchDomainsNoSearch = true
        settings.dnsSettings = dns
        settings.mtu = NSNumber(value: mtu)
        return settings
    }

    private func validatedAddress() throws -> (UInt32, Int) {
        let invalid = NativeSessionConfigurationError.invalid
        guard version == 1, mtu == 1280, let uuid = UUID(uuidString: generation),
              uuid != UUID(uuid: (0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0)),
              uuid.uuidString.lowercased() == generation else { throw invalid }
        let parts = address.split(separator: "/", omittingEmptySubsequences: false)
        guard parts.count == 2, let prefix = Int(parts[1]), (1...32).contains(prefix),
              String(prefix) == parts[1], let ip = Self.ipv4(String(parts[0])),
              Self.text(ip) == parts[0], (1...4).contains(dnsServers.count),
              (1...8).contains(dnsDomains.count) else { throw invalid }
        let mask = UInt32.max << (32 - prefix)
        for server in dnsServers {
            guard let dns = Self.ipv4(server), Self.text(dns) == server,
                  dns & mask == ip & mask else { throw invalid }
        }
        for domain in dnsDomains {
            guard !domain.isEmpty, domain.utf8.count <= 253 else { throw invalid }
            for label in domain.split(separator: ".", omittingEmptySubsequences: false) {
                let bytes = Array(label.utf8)
                guard (1...63).contains(bytes.count), bytes.first != 45, bytes.last != 45,
                      bytes.allSatisfy({ (65...90).contains($0) || (97...122).contains($0) || (48...57).contains($0) || $0 == 45 }) else { throw invalid }
            }
        }
        return (ip, prefix)
    }

    private static func ipv4(_ text: String) -> UInt32? {
        guard let ip = IPv4Address(text) else { return nil }
        let bytes = [UInt8](ip.rawValue)
        guard bytes.count == 4, bytes[0] != 0, bytes[0] != 127, bytes[0] < 224 else { return nil }
        return bytes.reduce(UInt32(0)) { ($0 << 8) | UInt32($1) }
    }
    private static func text(_ ip: UInt32) -> String {
        [24,16,8,0].map { String((ip >> $0) & 255) }.joined(separator: ".")
    }
}
