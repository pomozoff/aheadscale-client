import XCTest
@testable import AheadscaleTunnel

final class NativeReadinessGateTests: XCTestCase {
    func testConnectedRequiresAllThreePartsAndIsDeliveredOnce() {
        let generation = UUID()
        var ready = 0
        let gate = NativeReadinessGate(generation: generation, onReady: { ready += 1 }, onStop: { _ in XCTFail("unexpected stop") })
        gate.mark(.engine, generation: generation)
        gate.mark(.channel, generation: generation)
        XCTAssertEqual(ready, 0)
        gate.mark(.settings, generation: generation)
        gate.mark(.settings, generation: generation)
        XCTAssertEqual(ready, 1)
    }

    func testCancelRejectsLateAndOldGenerationResponses() {
        let generation = UUID()
        var stops = 0
        let gate = NativeReadinessGate(generation: generation, onReady: { XCTFail("Connected after Cancel") }, onStop: { _ in stops += 1 })
        for part in [NativeReadinessGate.Part.channel, .settings, .engine] { gate.mark(part, generation: UUID()) }
        XCTAssertEqual(stops, 0)
        gate.mark(.channel, generation: generation)
        gate.cancel()
        gate.cancel()
        gate.mark(.settings, generation: generation)
        gate.mark(.engine, generation: generation)
        XCTAssertEqual(stops, 1)
    }

    func testFailureAfterReadyDropsConnectionExactlyOnce() {
        let generation = UUID()
        var events: [String] = []
        let gate = NativeReadinessGate(generation: generation, onReady: { events.append("ready") }, onStop: { _ in events.append("stop") })
        for part in [NativeReadinessGate.Part.channel, .settings, .engine] { gate.mark(part, generation: generation) }
        let error = NSError(domain: "test", code: 1)
        gate.fail(error, generation: UUID())
        XCTAssertEqual(events, ["ready"])
        gate.fail(error, generation: generation)
        gate.fail(error, generation: generation)
        gate.mark(.engine, generation: generation)
        XCTAssertEqual(events, ["ready", "stop"])
    }

    func testReadyCallbackMayCancelReentrantly() {
        let generation = UUID()
        var stops = 0
        var gate: NativeReadinessGate!
        gate = NativeReadinessGate(generation: generation, onReady: { gate.cancel() }, onStop: { _ in stops += 1 })
        for part in [NativeReadinessGate.Part.channel, .settings, .engine] { gate.mark(part, generation: generation) }
        XCTAssertEqual(stops, 1)
        gate = nil
    }

    func testConcurrentReadyAndCancelNeverDeliverReadyAfterStop() {
        for _ in 0..<100 {
            let generation = UUID()
            var events: [String] = []
            let gate = NativeReadinessGate(generation: generation, onReady: { events.append("ready") }, onStop: { _ in events.append("stop") })
            DispatchQueue.concurrentPerform(iterations: 4) { index in
                if index == 3 { gate.cancel() }
                else { gate.mark([.channel, .settings, .engine][index], generation: generation) }
            }
            XCTAssertTrue(events == ["stop"] || events == ["ready", "stop"])
        }
    }
}
