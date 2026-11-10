import XCTest
import Foundation
@testable import AheadscaleBridgeProbe

final class NativeProbeTests: XCTestCase {
    func testOptionsRequireExplicitRoleAndBoundedDuration() throws {
        let client = try ProbeScenario.parse(["client", "--seconds", "120"])
        XCTAssertEqual(client.role, .client)
        XCTAssertEqual(client.seconds, 120)
        XCTAssertFalse(client.expectRefusal)
        XCTAssertTrue(try ProbeScenario.parse(["client", "--expect-refusal"]).expectRefusal)
        XCTAssertEqual(try ProbeScenario.parse(["server"]).role, .server)
        for args in [[], ["other"], ["client","--seconds","0"], ["client","--seconds","601"],
                     ["client","--seconds","abc"], ["client","--seconds"], ["server","--expect-refusal"],
                     ["client","--unknown"], ["client","--seconds","1","--seconds","2"],
                     ["client","--expect-refusal","--expect-refusal"]] {
            XCTAssertThrowsError(try ProbeScenario.parse(args), "Invalid arguments: \(args)")
        }
    }

    func testPacketsHaveCorrectIPLengthsChecksumAndSequence() {
        for sequence in UInt64(0)..<16 {
            let packet = ProbeScenario.packet(sequence: sequence)
            XCTAssertLessThanOrEqual(packet.count, 1280)
            XCTAssertGreaterThanOrEqual(packet.count, 64)
            XCTAssertEqual(packet[0] >> 4, sequence % 2 == 0 ? 4 : 6)
            if sequence % 2 == 0 {
                XCTAssertEqual(Int(packet[2]) << 8 | Int(packet[3]), packet.count)
                var sum: UInt32 = 0
                for index in stride(from: 0, to: 20, by: 2) {
                    sum += UInt32(packet[index]) << 8 | UInt32(packet[index + 1])
                }
                while sum > 65535 { sum = (sum & 65535) + (sum >> 16) }
                XCTAssertEqual(sum, 65535, "IPv4 checksum must be valid")
            } else {
                XCTAssertEqual(Int(packet[4]) << 8 | Int(packet[5]), packet.count - 40)
            }
            var recovered: UInt64 = 0
            for index in 0..<8 { recovered |= UInt64(packet[packet.count - 8 + index]) << (8 * index) }
            XCTAssertEqual(recovered, sequence)
        }
    }
}
