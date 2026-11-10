import XCTest
import Foundation
@testable import AheadscaleBridgeProbe

final class PacketFramingTests: XCTestCase {
    private func reader(_ wire: Data, chunk: Int = 1) -> (Int) throws -> Data {
        let stream = InputStream(data: wire)
        stream.open()
        return { count in
            var bytes = [UInt8](repeating: 0, count: min(count, chunk))
            let n = stream.read(&bytes, maxLength: bytes.count)
            guard n >= 0 else { throw stream.streamError! }
            return Data(bytes.prefix(n))
        }
    }

    func testPacketsPreservedAcrossShortIO() throws {
        let ipv4 = Data([0x45,0,0,28,0,1,0,0,64,1,0x66,0xde,10,0,0,1,10,0,0,2,8,0,0xf7,0xff,0,0,0,0])
        var ipv6 = Data(repeating: 0, count: 40)
        ipv6[0] = 0x60
        for packet in [ipv4, ipv6, Data(repeating: 0x45, count: 65535)] {
            let stream = OutputStream.toMemory()
            stream.open()
            defer { stream.close() }
            try PacketFraming.write(packet, mtu: 65535) { bytes in
                bytes.withUnsafeBytes { buffer in
                    stream.write(buffer.bindMemory(to: UInt8.self).baseAddress!, maxLength: min(bytes.count, 3))
                }
            }
            let wire = try XCTUnwrap(stream.property(forKey: .dataWrittenToMemoryStreamKey) as? Data)
            let header = Data([UInt8(truncatingIfNeeded: packet.count), UInt8(truncatingIfNeeded: packet.count >> 8), 0, 0])
            XCTAssertEqual(wire, header + packet, "Frame must use the existing Go little-endian format")
            XCTAssertEqual(try PacketFraming.read(mtu: 65535, read: reader(wire)), packet)
        }
    }

    func testMalformedFramesFail() {
        for wire in [Data([0,0,0,0]), Data([0,0,1,0]), Data([1,0]), Data([4,0,0,0,0x45,0])] {
            XCTAssertThrowsError(try PacketFraming.read(mtu: 65535, read: reader(wire)), "Malformed or truncated frame must fail")
        }
    }

    func testMTUBoundRejectsBeforeReadingBody() {
        var readCount = 0
        let prefix = Data([1,5,0,0])
        XCTAssertThrowsError(try PacketFraming.read(mtu: 1280) { count in
            readCount += 1
            XCTAssertEqual(count, 4, "Oversized body must not be requested")
            return prefix
        })
        XCTAssertEqual(readCount, 1, "Length must be rejected at the header")
    }

    func testBadWriteLengthDoesNotEmitBytes() {
        for (size, mtu) in [(0,1280),(1281,1280),(65536,65535),(1,0),(1,65536)] {
            var written = 0
            XCTAssertThrowsError(try PacketFraming.write(Data(repeating: 0x45, count: size), mtu: mtu) { data in
                written += data.count
                return data.count
            })
            XCTAssertEqual(written, 0, "Invalid frame must fail before its first byte")
        }
    }

    func testNoProgressAndInvalidAdaptersFail() {
        for result in [0, -1, 5] {
            XCTAssertThrowsError(try PacketFraming.write(Data([0x45]), mtu: 1280) { _ in result })
        }
        XCTAssertThrowsError(try PacketFraming.read(mtu: 1280) { count in Data(repeating: 0, count: count + 1) })
        XCTAssertThrowsError(try PacketFraming.read(mtu: 0) { _ in XCTFail("Invalid MTU must not read"); return Data() })
    }

    func testMaximumConfiguredMTUIsAccepted() throws {
        let packet = Data(repeating: 0x60, count: 1280)
        var wire = Data()
        try PacketFraming.write(packet, mtu: 1280) { bytes in wire.append(bytes); return bytes.count }
        XCTAssertEqual(try PacketFraming.read(mtu: 1280, read: reader(wire, chunk: 1280)), packet)
    }
}
