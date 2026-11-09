import XCTest
import NetworkExtension
@testable import AheadscaleTunnel

final class PacketTunnelProviderTests: XCTestCase {
    func testStartWithoutBridgeFailsInsteadOfReportingConnected() {
        let provider = PacketTunnelProvider()
        let completed = expectation(description: "start completes with unavailable bridge")
        completed.assertForOverFulfill = true
        provider.startTunnel(options: nil) { error in
            let failure = error as NSError?
            XCTAssertEqual(failure?.domain, "ru.aheadscale.client.tunnel")
            XCTAssertEqual(failure?.code, 1)
            XCTAssertTrue(failure?.localizedDescription.contains("Aheadscale") == true)
            completed.fulfill()
        }
        wait(for: [completed], timeout: 2)
    }

    func testStopWithoutSessionCompletes() {
        let provider = PacketTunnelProvider()
        let completed = expectation(description: "stop completes")
        completed.assertForOverFulfill = true
        provider.stopTunnel(with: .userInitiated) { completed.fulfill() }
        wait(for: [completed], timeout: 2)
    }
}
