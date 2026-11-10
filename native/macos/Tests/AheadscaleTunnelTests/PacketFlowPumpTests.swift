import XCTest
import Darwin
import NetworkExtension
import AheadscaleBridgeProbe
@testable import AheadscaleTunnel

final class TestPacketFlow: PacketFlowIO {
    private let lock = NSLock()
    private var pending: (@Sendable ([Data], [NSNumber]) -> Void)?
    var requested: XCTestExpectation?
    var received: XCTestExpectation?
    private(set) var packets: [Data] = []
    private(set) var families: [NSNumber] = []

    func readPackets(completionHandler: @escaping @Sendable ([Data], [NSNumber]) -> Void) {
        lock.lock()
        pending = completionHandler
        let requested = self.requested
        self.requested = nil
        lock.unlock()
        requested?.fulfill()
    }
    func writePackets(_ packets: [Data], withProtocols protocols: [NSNumber]) -> Bool {
        lock.lock()
        self.packets = packets
        families = protocols
        lock.unlock()
        received?.fulfill()
        return true
    }
    func deliver(_ data: [Data], families: [NSNumber]) {
        lock.lock()
        let callback = pending
        pending = nil
        lock.unlock()
        callback?(data, families)
    }
}

final class PacketFlowPumpTests: XCTestCase {
    private func pair() throws -> (PacketSocket, PacketSocket) {
        var descriptors: [Int32] = [-1, -1]
        XCTAssertEqual(socketpair(AF_UNIX, SOCK_STREAM, 0, &descriptors), 0)
        defer { Darwin.close(descriptors[0]); Darwin.close(descriptors[1]) }
        return (try PacketSocket(duplicating: descriptors[0]), try PacketSocket(duplicating: descriptors[1]))
    }

    func testIdleSocketWaitDoesNotConsumeFrameOrCloseSession() throws {
        let (local, peer) = try pair()
        defer { local.shutdown(); peer.shutdown() }
        XCTAssertFalse(try local.waitForReadable(timeoutMilliseconds: 20))
        let packet = ProbeScenario.packet(sequence: 0)
        try peer.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 200)
        XCTAssertTrue(try local.waitForReadable(timeoutMilliseconds: 200))
        XCTAssertEqual(try local.readFrame(mtu: 1280, timeoutMilliseconds: 200), packet)
    }

    func testDuplexMovesActualFramesAndInfersInboundFamily() throws {
        let (local, peer) = try pair()
        defer { peer.shutdown() }
        let flow = TestPacketFlow()
        flow.requested = expectation(description: "flow read requested")
        flow.received = expectation(description: "inbound flow write")
        let requested = flow.requested!
        let received = flow.received!
        let pump = PacketFlowPump(flow: flow, socket: local, mtu: 1280) { _ in }
        defer { pump.stop() }
        pump.start()
        wait(for: [requested], timeout: 2)
        let ipv4 = ProbeScenario.packet(sequence: 0)
        flow.deliver([ipv4], families: [NSNumber(value: AF_INET)])
        XCTAssertEqual(try peer.readFrame(mtu: 1280, timeoutMilliseconds: 1000), ipv4)
        let ipv6 = ProbeScenario.packet(sequence: 1)
        try peer.writeFrame(ipv6, mtu: 1280, timeoutMilliseconds: 1000)
        wait(for: [received], timeout: 2)
        XCTAssertEqual(flow.packets, [ipv6])
        XCTAssertEqual(flow.families, [NSNumber(value: AF_INET6)])
    }

    func testCancelIgnoresLatePacketFlowCallbackAndFinishesOnce() throws {
        let (local, peer) = try pair()
        defer { peer.shutdown() }
        let flow = TestPacketFlow()
        flow.requested = expectation(description: "pending read")
        let requested = flow.requested!
        let stopped = expectation(description: "stopped once")
        stopped.assertForOverFulfill = true
        let pump = PacketFlowPump(flow: flow, socket: local, mtu: 1280) { error in
            XCTAssertNil(error)
            stopped.fulfill()
        }
        pump.start()
        wait(for: [requested], timeout: 2)
        pump.stop()
        flow.deliver([ProbeScenario.packet(sequence: 0)], families: [NSNumber(value: AF_INET)])
        pump.stop()
        pump.start()
        wait(for: [stopped], timeout: 2)
        XCTAssertThrowsError(try peer.readFrame(mtu: 1280, timeoutMilliseconds: 200))
    }

    func testOutboundProtocolMismatchClosesChannel() throws {
        let (local, peer) = try pair()
        defer { peer.shutdown() }
        let flow = TestPacketFlow()
        flow.requested = expectation(description: "pending read")
        let requested = flow.requested!
        let failed = expectation(description: "invalid family")
        failed.assertForOverFulfill = true
        let pump = PacketFlowPump(flow: flow, socket: local, mtu: 1280) { error in
            XCTAssertNotNil(error)
            failed.fulfill()
        }
        pump.start()
        wait(for: [requested], timeout: 2)
        flow.deliver([ProbeScenario.packet(sequence: 0)], families: [NSNumber(value: AF_INET6)])
        wait(for: [failed], timeout: 2)
        XCTAssertThrowsError(try peer.readFrame(mtu: 1280, timeoutMilliseconds: 200))
    }
    func testOnlyOwnIPv4SourceAndDestinationAreLocallyDelivered() throws {
        for addresses: [UInt8] in [[100,88,1,3,100,88,1,2],
                                    [100,88,1,2,100,88,1,3],
                                    [100,88,1,3,100,88,1,3]] {
            let (local, peer) = try pair()
            defer { peer.shutdown() }
            let flow = TestPacketFlow()
            let requested = expectation(description: "read requested")
            flow.requested = requested
            let pump = PacketFlowPump(flow: flow, socket: local, mtu: 1280,
                localIPv4: Data([100,88,1,2])) { _ in }
            defer { pump.stop() }
            pump.start(); wait(for: [requested], timeout: 2)
            var packet = ProbeScenario.packet(sequence: 2)
            packet.replaceSubrange(12..<20, with: addresses)
            flow.deliver([packet], families: [NSNumber(value: AF_INET)])
            XCTAssertEqual(try peer.readFrame(mtu: 1280, timeoutMilliseconds: 1000), packet)
            XCTAssertTrue(flow.packets.isEmpty)
        }
    }

    func testCancelIgnoresLateSelfPacketAndMalformedSelfPacketFails() throws {
        for cancel in [true, false] {
            let (local, peer) = try pair()
            defer { peer.shutdown() }
            let flow = TestPacketFlow()
            let requested = expectation(description: "read requested")
            let stopped = expectation(description: "stopped once")
            stopped.assertForOverFulfill = true
            flow.requested = requested
            let pump = PacketFlowPump(flow: flow, socket: local, mtu: 1280,
                localIPv4: Data([100,88,1,2])) { error in
                    XCTAssertEqual(error == nil, cancel); stopped.fulfill()
                }
            pump.start(); wait(for: [requested], timeout: 2)
            var packet = ProbeScenario.packet(sequence: 2)
            packet.replaceSubrange(12..<20, with: [100,88,1,2,100,88,1,2])
            if cancel { pump.stop() } else { packet[0] = 0x4f }
            flow.deliver([packet], families: [NSNumber(value: AF_INET)])
            wait(for: [stopped], timeout: 2)
            XCTAssertTrue(flow.packets.isEmpty)
            XCTAssertThrowsError(try peer.readFrame(mtu: 1280, timeoutMilliseconds: 100))
        }
    }

}
