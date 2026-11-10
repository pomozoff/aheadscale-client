import XCTest
import NetworkExtension
@testable import AheadscaleActivation

@MainActor
final class PrototypeVPNTests: XCTestCase {
    private func profile(marker: Bool = true, identifier: String = "ru.aheadscale.client.tunnel") -> NETunnelProviderManager {
        let manager = NETunnelProviderManager()
        let configuration = NETunnelProviderProtocol()
        configuration.providerBundleIdentifier = identifier
        if marker { configuration.providerConfiguration = ["aheadscale.prototype": "session-v1"] }
        manager.protocolConfiguration = configuration
        return manager
    }

    func testSelectOnlyOwnedPrototypeAndRejectAmbiguousDuplicates() throws {
        let foreign = profile(identifier: "com.example.vpn")
        let unmarked = profile(marker: false)
        let owned = profile()
        XCTAssertTrue(try PrototypeVPNController.select([foreign, unmarked, owned]) === owned)
        XCTAssertNil(try PrototypeVPNController.select([foreign, unmarked]))
        XCTAssertThrowsError(try PrototypeVPNController.select([owned, profile()]))
    }

    func testNewProfileHasNoAutoconnectOrDefaultRouteConfiguration() {
        let manager = PrototypeVPNController.makeProfile()
        XCTAssertEqual(manager.localizedDescription, "Aheadscale — тест системного VPN")
        XCTAssertFalse(manager.isOnDemandEnabled)
        XCTAssertTrue(manager.isEnabled)
        let configuration = manager.protocolConfiguration as? NETunnelProviderProtocol
        XCTAssertEqual(configuration?.providerBundleIdentifier, "ru.aheadscale.client.tunnel")
        XCTAssertEqual(configuration?.providerConfiguration?["aheadscale.prototype"] as? String, "session-v1")
        XCTAssertEqual(configuration?.serverAddress, "aheadscale.ru")
        XCTAssertFalse(configuration?.includeAllNetworks ?? true)
    }

    func testConnectSavesReloadsThenStartsOnceForRepeatedClick() async {
        let finished = expectation(description: "started")
        var calls: [String] = []
        let owned = profile()
        let controller = PrototypeVPNController(operations: .init(
            load: { calls.append("load"); await Task.yield(); return [owned] },
            save: { _ in calls.append("save") },
            reload: { _ in calls.append("reload") },
            start: { _ in calls.append("start"); finished.fulfill() },
            stop: { _ in calls.append("stop") },
            remove: { _ in calls.append("remove") }
        ))
        controller.connect()
        controller.connect()
        await fulfillment(of: [finished], timeout: 2)
        XCTAssertEqual(calls, ["load", "save", "reload", "start"])
        XCTAssertEqual(controller.state, .starting)
    }

    func testCancelWhilePreferencesReloadCannotStartOldAttempt() async {
        let loaded = expectation(description: "reload entered")
        let completed = expectation(description: "old attempt released")
        var release: CheckedContinuation<Void, Never>?
        var starts = 0
        var stops = 0
        let owned = profile()
        let controller = PrototypeVPNController(operations: .init(
            load: { [owned] }, save: { _ in },
            reload: { _ in
                await withCheckedContinuation { release = $0; loaded.fulfill() }
                completed.fulfill()
            },
            start: { _ in starts += 1 }, stop: { _ in stops += 1 }, remove: { _ in }
        ))
        controller.connect()
        await fulfillment(of: [loaded], timeout: 2)
        controller.disconnect()
        release?.resume()
        await fulfillment(of: [completed], timeout: 2)
        await Task.yield()
        XCTAssertEqual(starts, 0)
        XCTAssertEqual(stops, 1)
        XCTAssertEqual(controller.state, .disconnected)
        controller.consume(.connecting)
        XCTAssertEqual(controller.state, .disconnected, "Late status must not undo Cancel")
    }

    func testPreferencesFailureNeverStartsOrReportsConnected() async {
        let failed = expectation(description: "failed")
        var starts = 0
        let controller = PrototypeVPNController(operations: .init(
            load: { throw NSError(domain: "preferences", code: 7) },
            save: { _ in }, reload: { _ in }, start: { _ in starts += 1 },
            stop: { _ in }, remove: { _ in }
        ))
        controller.onChange = { state in if case .failed = state { failed.fulfill() } }
        controller.connect()
        await fulfillment(of: [failed], timeout: 2)
        XCTAssertEqual(starts, 0)
    }

    func testUnexpectedConnectedWithoutEngineIsStopped() async {
        let finished = expectation(description: "start called")
        var stops = 0
        let owned = profile()
        let controller = PrototypeVPNController(operations: .init(
            load: { [owned] }, save: { _ in }, reload: { _ in },
            start: { _ in finished.fulfill() }, stop: { _ in stops += 1 }, remove: { _ in }
        ))
        controller.connect()
        await fulfillment(of: [finished], timeout: 2)
        controller.consume(.connected)
        XCTAssertEqual(stops, 1)
        guard case .failed = controller.state else { return XCTFail("No engine: must not report Connected") }
    }
}

@MainActor
final class NativeProfileTests: XCTestCase {
    func testEngineProfileSelectsLiveProviderWithoutProofFallback() {
        let manager=PrototypeVPNController.makeProfile(nativeEngine:true)
        let config=manager.protocolConfiguration as? NETunnelProviderProtocol
        XCTAssertEqual(config?.providerConfiguration?["aheadscale.native-engine"] as? Bool,true)
        XCTAssertNil(config?.providerConfiguration?["aheadscale.bridge-proof"])
        XCTAssertFalse(config?.includeAllNetworks ?? true)
        XCTAssertFalse(manager.isOnDemandEnabled)
    }
}
