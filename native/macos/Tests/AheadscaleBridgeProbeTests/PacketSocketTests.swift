import XCTest
import Foundation
import Darwin
@testable import AheadscaleBridgeProbe

final class PacketSocketTests: XCTestCase {
    private func pair() throws -> (PacketSocket, PacketSocket) {
        var descriptors: [Int32] = [-1, -1]
        guard socketpair(AF_UNIX, SOCK_STREAM, 0, &descriptors) == 0 else {
            throw NSError(domain: NSPOSIXErrorDomain, code: Int(errno))
        }
        defer { Darwin.close(descriptors[0]); Darwin.close(descriptors[1]) }
        var size: Int32 = 1024
        XCTAssertEqual(setsockopt(descriptors[0], SOL_SOCKET, SO_SNDBUF, &size,
                                  socklen_t(MemoryLayout<Int32>.size)), 0)
        return (try PacketSocket(duplicating: descriptors[0]), try PacketSocket(duplicating: descriptors[1]))
    }

    func testDuplexFramesThroughRealSockets() throws {
        let (client, server) = try pair()
        defer { client.shutdown(); server.shutdown() }
        let echoed = expectation(description: "server echoed packet")
        let packet = Data(repeating: 0x60, count: 1280)
        DispatchQueue.global().async {
            do {
                let received = try server.readFrame(mtu: 1280, timeoutMilliseconds: 2000)
                XCTAssertEqual(received, packet)
                try server.writeFrame(received, mtu: 1280, timeoutMilliseconds: 2000)
            } catch { XCTFail("Socket echo failed: \(error)") }
            echoed.fulfill()
        }
        try client.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 2000)
        XCTAssertEqual(try client.readFrame(mtu: 1280, timeoutMilliseconds: 2000), packet)
        wait(for: [echoed], timeout: 3)
    }

    func testIdleReadHasDeadline() throws {
        let (client, server) = try pair()
        defer { client.shutdown(); server.shutdown() }
        XCTAssertThrowsError(try client.readFrame(mtu: 1280, timeoutMilliseconds: 20)) {
            XCTAssertEqual($0 as? PacketSocketError, .timeout)
        }
    }

    func testNonReadingPeerCannotBlockWritesIndefinitely() throws {
        let (client, server) = try pair()
        defer { client.shutdown(); server.shutdown() }
        var timedOut = false
        for _ in 0..<128 {
            do {
                try client.writeFrame(Data(repeating: 0x45, count: 1280), mtu: 1280, timeoutMilliseconds: 50)
            } catch {
                XCTAssertEqual(error as? PacketSocketError, .timeout)
                timedOut = true
                break
            }
        }
        XCTAssertTrue(timedOut, "Backpressure must produce a bounded timeout")
    }

    func testShutdownUnblocksWaitingReader() throws {
        let (client, server) = try pair()
        defer { client.shutdown(); server.shutdown() }
        let started = expectation(description: "reader started")
        let stopped = expectation(description: "reader unblocked")
        DispatchQueue.global().async {
            started.fulfill()
            do {
                _ = try client.readFrame(mtu: 1280, timeoutMilliseconds: 5000)
                XCTFail("Shutdown must not return a packet")
            } catch {
                XCTAssertNotEqual(error as? PacketSocketError, .timeout, "Shutdown must precede deadline")
            }
            stopped.fulfill()
        }
        wait(for: [started], timeout: 1)
        server.shutdown()
        wait(for: [stopped], timeout: 1)
    }

    func testInvalidDescriptorAndDatagramAreRejected() throws {
        XCTAssertThrowsError(try PacketSocket(duplicating: -1))
        var descriptors: [Int32] = [-1, -1]
        XCTAssertEqual(socketpair(AF_UNIX, SOCK_DGRAM, 0, &descriptors), 0)
        defer { Darwin.close(descriptors[0]); Darwin.close(descriptors[1]) }
        XCTAssertThrowsError(try PacketSocket(duplicating: descriptors[0]))
    }
}
