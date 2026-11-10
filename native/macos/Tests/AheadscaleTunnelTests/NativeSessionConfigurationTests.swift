import XCTest
import NetworkExtension
@testable import AheadscaleTunnel

final class NativeSessionConfigurationTests: XCTestCase {
    private let json = #"{"version":1,"generation":"d75aef4f-7e19-4ae4-a841-6c250fc775c2","address":"100.88.19.208/16","mtu":1280,"dnsServers":["100.88.255.254"],"dnsDomains":["aheadscale.internal"]}"#

    func testSameGoContractBuildsOnlyOverlaySettings() throws {
        let config = try NativeSessionConfiguration.parse(Data(json.utf8))
        let settings = try config.networkSettings()
        XCTAssertEqual(settings.ipv4Settings?.addresses, ["100.88.19.208"])
        XCTAssertEqual(settings.ipv4Settings?.subnetMasks, ["255.255.0.0"])
        XCTAssertEqual(settings.ipv4Settings?.includedRoutes?.count, 1)
        XCTAssertEqual(settings.ipv4Settings?.includedRoutes?.first?.destinationAddress, "100.88.0.0")
        XCTAssertEqual(settings.ipv4Settings?.includedRoutes?.first?.destinationSubnetMask, "255.255.0.0")
        XCTAssertEqual(settings.dnsSettings?.servers, ["100.88.255.254"])
        XCTAssertEqual(settings.dnsSettings?.matchDomains, ["aheadscale.internal"])
        XCTAssertEqual(settings.mtu?.intValue, 1280)
    }

    func testRejectsDefaultRouteBadGenerationForeignDNSAndSecrets() {
        let invalid = [
            json.replacingOccurrences(of: #""version":1"#, with: #""version":2"#),
            json.replacingOccurrences(of: "d75aef4f-7e19-4ae4-a841-6c250fc775c2", with: "old-session"),
            json.replacingOccurrences(of: "100.88.19.208/16", with: "100.88.19.208/0"),
            json.replacingOccurrences(of: "100.88.19.208/16", with: "127.0.0.1/8"),
            json.replacingOccurrences(of: "100.88.19.208/16", with: "::1/128"),
            json.replacingOccurrences(of: #""mtu":1280"#, with: #""mtu":0"#),
            json.replacingOccurrences(of: "100.88.255.254", with: "8.8.8.8"),
            json.replacingOccurrences(of: "aheadscale.internal", with: "*"),
            json.replacingOccurrences(of: "aheadscale.internal", with: "-invalid.example"),
            json.replacingOccurrences(of: #""version":1"#, with: #""privateKey":"secret","version":1"#),
            json + "{}", String(repeating: " ", count: 4097)
        ]
        for data in invalid { XCTAssertThrowsError(try NativeSessionConfiguration.parse(Data(data.utf8))) }
    }
}
