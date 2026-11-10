import Foundation

enum PacketFrameError: Error {
    case invalidMTU, invalidLength, truncated, invalidIO, noProgress
}

enum PacketFraming {
    static let maximumLength = 65535

    static func read(mtu: Int, read: (Int) throws -> Data) throws -> Data {
        try validate(mtu: mtu)
        let header = try readExactly(4, read: read)
        let length = header.enumerated().reduce(0) { $0 | (Int($1.element) << (8 * $1.offset)) }
        guard length > 0, length <= mtu, length <= maximumLength else {
            throw PacketFrameError.invalidLength
        }
        // Validate the untrusted prefix before allocating or requesting its payload.
        return try readExactly(length, read: read)
    }

    static func write(_ packet: Data, mtu: Int, write: (Data) throws -> Int) throws {
        try validate(mtu: mtu)
        guard !packet.isEmpty, packet.count <= mtu, packet.count <= maximumLength else {
            throw PacketFrameError.invalidLength
        }
        let header = Data((0..<4).map { UInt8(truncatingIfNeeded: packet.count >> (8 * $0)) })
        try writeAll(header, write: write)
        try writeAll(packet, write: write)
    }

    private static func validate(mtu: Int) throws {
        guard mtu > 0, mtu <= maximumLength else { throw PacketFrameError.invalidMTU }
    }

    private static func readExactly(_ count: Int, read: (Int) throws -> Data) throws -> Data {
        var output = Data()
        output.reserveCapacity(count)
        while output.count < count {
            let remaining = count - output.count
            let chunk = try read(remaining)
            guard chunk.count <= remaining else { throw PacketFrameError.invalidIO }
            guard !chunk.isEmpty else { throw PacketFrameError.truncated }
            output.append(chunk)
        }
        return output
    }

    private static func writeAll(_ data: Data, write: (Data) throws -> Int) throws {
        var offset = 0
        while offset < data.count {
            let remaining = data.count - offset
            let accepted = try write(Data(data.dropFirst(offset)))
            guard accepted >= 0, accepted <= remaining else { throw PacketFrameError.invalidIO }
            guard accepted > 0 else { throw PacketFrameError.noProgress }
            offset += accepted
        }
    }
}
