import XCTest
import Foundation
import Darwin
@testable import AheadscaleBridgeProbe

final class NativeDescriptorHandoffTests: XCTestCase {
    func testBadGenerationCannotSendDescriptor() {
        XCTAssertThrowsError(try NativeDescriptorHandoff.send(packetFD: -1, generation: "old-session", controlFD: -1)) { error in
            XCTAssertEqual(error as? NativeDescriptorHandoffError, .invalidGeneration)
        }
    }

    func testSwiftTransfersStreamToRealGoProcess() throws {
        guard let helper = ProcessInfo.processInfo.environment["AHEADSCALE_HANDOFF_CHECK"], !helper.isEmpty else {
            throw XCTSkip("Межъязыковой прогон выполняется отдельным macOS handoff workflow")
        }
        var controls: [Int32] = [-1, -1]
        var packets: [Int32] = [-1, -1]
        XCTAssertEqual(socketpair(AF_UNIX, SOCK_DGRAM, 0, &controls), 0)
        XCTAssertEqual(socketpair(AF_UNIX, SOCK_STREAM, 0, &packets), 0)
        defer { for fd in controls + packets where fd >= 0 { Darwin.close(fd) } }
        // All originals are close-on-exec; Process intentionally maps only the
        // control endpoint to stdin. The packet FD can reach Go only via rights.
        for fd in controls + packets { XCTAssertEqual(fcntl(fd, F_SETFD, FD_CLOEXEC), 0) }
        let generation = UUID().uuidString.lowercased()
        let process = Process()
        process.executableURL = URL(fileURLWithPath: helper)
        process.arguments = [generation]
        let childInput = FileHandle(fileDescriptor: controls[1], closeOnDealloc: false)
        process.standardInput = childInput
        process.standardOutput = Pipe()
        process.standardError = Pipe()
        defer { if process.isRunning { process.terminate(); process.waitUntilExit() } }
        try process.run()
        try childInput.close()
        controls[1] = -1
        let peer = try PacketSocket(duplicating: packets[1])
        defer { peer.shutdown() }
        try NativeDescriptorHandoff.send(packetFD: packets[0], generation: generation, controlFD: controls[0])
        Darwin.close(packets[0]); packets[0] = -1
        for sequence: UInt64 in 0..<2 {
            let packet = ProbeScenario.packet(sequence: sequence)
            try peer.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 2000)
            XCTAssertEqual(try peer.readFrame(mtu: 1280, timeoutMilliseconds: 2000), packet)
        }
        process.waitUntilExit()
        XCTAssertEqual(process.terminationStatus, 0)
    }
}
