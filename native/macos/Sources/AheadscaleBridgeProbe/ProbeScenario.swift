import Foundation

public enum ProbeRole: String, Equatable { case server, client }
public enum ProbeArgumentError: Error { case invalidArguments }

public struct ProbeScenario {
    public let role: ProbeRole
    public let seconds: Int
    public let expectRefusal: Bool
    public static let serviceName = "ru.aheadscale.probe.20261110"
    public static let serverRequirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.probe.server\""
    public static let clientRequirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.probe.client\""

    public static func parse(_ arguments: [String]) throws -> ProbeScenario {
        guard let first = arguments.first, let role = ProbeRole(rawValue: first) else {
            throw ProbeArgumentError.invalidArguments
        }
        var seconds = role == .server ? 300 : 120
        var refusal = false
        var seen = Set<String>()
        var index = 1
        while index < arguments.count {
            let flag = arguments[index]
            guard seen.insert(flag).inserted else { throw ProbeArgumentError.invalidArguments }
            switch flag {
            case "--seconds":
                index += 1
                guard index < arguments.count, let value = Int(arguments[index]),
                      (1...600).contains(value) else { throw ProbeArgumentError.invalidArguments }
                seconds = value
            case "--expect-refusal":
                guard role == .client else { throw ProbeArgumentError.invalidArguments }
                refusal = true
            default: throw ProbeArgumentError.invalidArguments
            }
            index += 1
        }
        return ProbeScenario(role: role, seconds: seconds, expectRefusal: refusal)
    }

    public static func packet(sequence: UInt64) -> Data {
        let ipv4 = sequence % 2 == 0
        let count = sequence % 3 == 0 ? 1280 : 64
        let header = ipv4 ? 20 : 40
        var packet = Data(repeating: 0, count: count)
        for index in header..<count { packet[index] = UInt8(truncatingIfNeeded: sequence &+ UInt64(index)) }
        for index in 0..<8 { packet[count - 8 + index] = UInt8(truncatingIfNeeded: sequence >> (8 * index)) }
        if ipv4 {
            packet[0] = 0x45
            packet[2] = UInt8(truncatingIfNeeded: count >> 8)
            packet[3] = UInt8(truncatingIfNeeded: count)
            packet[8] = 64
            packet[9] = 253
            packet.replaceSubrange(12..<20, with: [192,0,2,1,192,0,2,2])
            var sum: UInt32 = 0
            for index in stride(from: 0, to: 20, by: 2) {
                sum += UInt32(packet[index]) << 8 | UInt32(packet[index + 1])
            }
            while sum > 65535 { sum = (sum & 65535) + (sum >> 16) }
            let checksum = ~UInt16(sum)
            packet[10] = UInt8(truncatingIfNeeded: checksum >> 8)
            packet[11] = UInt8(truncatingIfNeeded: checksum)
        } else {
            packet[0] = 0x60
            packet[4] = UInt8(truncatingIfNeeded: (count - 40) >> 8)
            packet[5] = UInt8(truncatingIfNeeded: count - 40)
            packet[6] = 253
            packet[7] = 64
            packet.replaceSubrange(8..<12, with: [0x20,0x01,0x0d,0xb8])
            packet.replaceSubrange(24..<28, with: [0x20,0x01,0x0d,0xb8])
            packet[23] = 1
            packet[39] = 2
        }
        return packet
    }
}
