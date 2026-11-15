import XCTest
import Foundation
import NetworkExtension
import Darwin
@testable import AheadscaleActivation

@MainActor
final class HeadlessNotificationTests: XCTestCase {
    private static var retained: PrototypeVPNController?

    func testDispatchMainNotificationDoesNotDeadlockControlProcess() async throws {
        let child = Process()
        child.executableURL = URL(fileURLWithPath: "/usr/bin/xcrun")
        child.arguments = ["xctest", "-XCTest",
            "AheadscaleActivationTests.HeadlessNotificationTests/testNotificationChild",
            Bundle(for: Self.self).bundleURL.path]
        var environment = ProcessInfo.processInfo.environment
        environment["AHEADSCALE_HEADLESS_NOTIFICATION_CHILD"] = "1"
        child.environment = environment
        let output = Pipe()
        child.standardOutput = output; child.standardError = output
        let completed = expectation(description: "headless notification completed")
        child.terminationHandler = { _ in completed.fulfill() }
        try child.run()
        defer { if child.isRunning { child.terminate() } }
        await fulfillment(of: [completed], timeout: 10)
        guard !child.isRunning else { return XCTFail("headless child did not terminate") }
        let text = String(decoding: output.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
        XCTAssertEqual(child.terminationStatus, 0, String(text.suffix(1500)))
        XCTAssertTrue(text.contains("HEADLESS_NOTIFICATION_DELIVERED"), String(text.suffix(1500)))
    }

    func testNotificationChild() {
        guard ProcessInfo.processInfo.environment["AHEADSCALE_HEADLESS_NOTIFICATION_CHILD"] == "1" else { return }
        guard Thread.isMainThread else { exit(4) }
        // Bound the known pre-fix deadlock without hanging the parent XCTest.
        alarm(3)
        Task { @MainActor in
            let owned = PrototypeVPNController.makeProfile(nativeEngine: true)
            let controller = PrototypeVPNController(operations: .init(
                load: { [owned] }, save: { _ in }, reload: { _ in },
                start: { manager in
                    NotificationCenter.default.post(name: .NEVPNStatusDidChange,
                        object: manager.connection)
                }, stop: { _ in }, remove: { _ in }
            ), nativeEngine: true)
            Self.retained = controller
            controller.onChange = { state in
                if case .failed = state {
                    print("HEADLESS_NOTIFICATION_DELIVERED")
                    fflush(stdout); exit(0)
                }
            }
            controller.connect()
        }
        dispatchMain()
    }
}
