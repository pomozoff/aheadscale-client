import XCTest
@testable import AheadscaleTunnel

final class ProviderBridgeProofTests: XCTestCase {
    func testMissingRootBrokerCannotReportReady() {
        let finished = expectation(description: "missing broker fails")
        finished.assertForOverFulfill = true
        let proof = ProviderBridgeProof { result in
            if case .success = result { XCTFail("A missing root broker must not report ready") }
            finished.fulfill()
        }
        proof.start()
        wait(for: [finished], timeout: 8)
    }

    func testCancelBeforeStartCompletesOnceAndCannotReopen() {
        let finished = expectation(description: "cancelled once")
        finished.assertForOverFulfill = true
        let proof = ProviderBridgeProof { result in
            if case .success = result { XCTFail("Cancelled proof must not report ready") }
            finished.fulfill()
        }
        proof.stop()
        proof.start()
        proof.stop()
        wait(for: [finished], timeout: 2)
    }
}
