import XCTest
@testable import AheadscaleTunnel

final class PacketTunnelProviderTests: XCTestCase {
    func testStartWithoutBridgeFailsInsteadOfReportingConnected() {
        let provider = TunnelLifecycle()
        let completed = expectation(description: "start completes with unavailable bridge")
        completed.assertForOverFulfill = true
        provider.start() { error in
            let failure = error as NSError?
            XCTAssertEqual(failure?.domain, "ru.aheadscale.client.tunnel")
            XCTAssertEqual(failure?.code, 1)
            XCTAssertTrue(failure?.localizedDescription.contains("Aheadscale") == true)
            completed.fulfill()
        }
        wait(for: [completed], timeout: 2)
    }

    func testStopWithoutSessionCompletes() {
        let provider = TunnelLifecycle()
        let completed = expectation(description: "stop completes")
        completed.assertForOverFulfill = true
        provider.stop { completed.fulfill() }
        wait(for: [completed], timeout: 2)
    }

    private final class Proof: ProviderProofSession {
        var starts = 0
        var stops = 0
        var onStart: (() -> Void)?
        func start() { starts += 1; onStart?() }
        func stop() { stops += 1 }
    }

    func testSuccessfulProofStillCannotReportVPNConnected() {
        let lifecycle = TunnelLifecycle()
        let proof = Proof()
        var result: NSError?
        lifecycle.start(makeProof: { completed in
            proof.onStart = { completed(.success(())) }
            return proof
        }) { result = $0 as NSError? }
        XCTAssertEqual(result?.code, 2)
        XCTAssertTrue(result?.localizedDescription.contains("движок") == true)
    }

    func testCancelDuringFactoryDoesNotStartOrCompleteTwice() {
        let lifecycle = TunnelLifecycle()
        let proof = Proof()
        var completions = 0
        lifecycle.start(makeProof: { completed in
            lifecycle.stop {}
            proof.onStart = { completed(.success(())) }
            return proof
        }) { error in
            XCTAssertNotNil(error)
            completions += 1
        }
        XCTAssertEqual(proof.starts, 0)
        XCTAssertEqual(proof.stops, 1)
        XCTAssertEqual(completions, 1)
    }

    func testDuplicateStartDoesNotConstructSecondProof() {
        let lifecycle = TunnelLifecycle()
        let proof = Proof()
        var factories = 0
        lifecycle.start(makeProof: { _ in factories += 1; return proof }) { _ in }
        var duplicate: NSError?
        lifecycle.start(makeProof: { _ in factories += 1; return proof }) { duplicate = $0 as NSError? }
        XCTAssertEqual(factories, 1)
        XCTAssertEqual(duplicate?.code, 3)
        lifecycle.stop {}
    }
}
