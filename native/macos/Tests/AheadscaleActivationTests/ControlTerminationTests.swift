import XCTest
import Dispatch
import Darwin
@testable import AheadscaleActivation

final class ControlTerminationTests: XCTestCase {
    func testTerminationSignalReachesControlCancellation() async {
        let received = expectation(description: "own command cancellation")
        let previous = signal(SIGTERM, SIG_IGN)
        let source = VPNControlTermination.install { received.fulfill() }
        defer { source.cancel(); signal(SIGTERM, previous) }
        raise(SIGTERM)
        await fulfillment(of: [received], timeout: 2)
    }
}
