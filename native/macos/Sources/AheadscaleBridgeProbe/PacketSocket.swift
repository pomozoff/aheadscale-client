import Foundation
import Darwin

enum PacketSocketError: Error, Equatable {
    case timeout, invalidTimeout, unsupportedSocket
    case system(Int32)
}

// One worker reads and one worker writes; cancellation uses shutdown, never close.
final class PacketSocket {
    private let descriptor: Int32

    init(duplicating source: Int32) throws {
        descriptor = try Self.prepareDescriptor(source)
    }

    func readFrame(mtu: Int, timeoutMilliseconds: Int) throws -> Data {
        let deadline = try deadline(after: timeoutMilliseconds)
        return try PacketFraming.read(mtu: mtu) { try self.readChunk($0, until: deadline) }
    }

    func writeFrame(_ packet: Data, mtu: Int, timeoutMilliseconds: Int) throws {
        let deadline = try deadline(after: timeoutMilliseconds)
        try PacketFraming.write(packet, mtu: mtu) { try self.writeChunk($0, until: deadline) }
    }

    func shutdown() {
        if Darwin.shutdown(descriptor, SHUT_RDWR) != 0, errno != ENOTCONN, errno != EINVAL {
            NSLog("Aheadscale probe socket shutdown failed: %d", errno)
        }
    }

    deinit {
        if Darwin.close(descriptor) != 0 {
            NSLog("Aheadscale probe socket close failed: %d", errno)
        }
    }

    private static func prepareDescriptor(_ source: Int32) throws -> Int32 {
        let copy = dup(source)
        guard copy >= 0 else { throw PacketSocketError.system(errno) }
        do {
            var kind: Int32 = 0
            var size = socklen_t(MemoryLayout<Int32>.size)
            guard getsockopt(copy, SOL_SOCKET, SO_TYPE, &kind, &size) == 0 else {
                throw PacketSocketError.system(errno)
            }
            guard kind == SOCK_STREAM else { throw PacketSocketError.unsupportedSocket }
            let flags = fcntl(copy, F_GETFL)
            guard flags >= 0, fcntl(copy, F_SETFL, flags | O_NONBLOCK) == 0 else {
                throw PacketSocketError.system(errno)
            }
            var enabled: Int32 = 1
            guard setsockopt(copy, SOL_SOCKET, SO_NOSIGPIPE, &enabled,
                             socklen_t(MemoryLayout<Int32>.size)) == 0 else {
                throw PacketSocketError.system(errno)
            }
            return copy
        } catch {
            if Darwin.close(copy) != 0 { NSLog("Aheadscale probe setup cleanup failed: %d", errno) }
            throw error
        }
    }

    private func deadline(after milliseconds: Int) throws -> UInt64 {
        guard (1...60000).contains(milliseconds) else { throw PacketSocketError.invalidTimeout }
        return DispatchTime.now().uptimeNanoseconds + UInt64(milliseconds) * 1_000_000
    }

    private func waitFor(_ event: Int16, until deadline: UInt64) throws {
        while true {
            let now = DispatchTime.now().uptimeNanoseconds
            guard now < deadline else { throw PacketSocketError.timeout }
            let remaining = Int32(min((deadline - now + 999_999) / 1_000_000, 60000))
            var item = pollfd(fd: descriptor, events: event, revents: 0)
            let ready = poll(&item, 1, remaining)
            if ready > 0 {
                guard item.revents & Int16(POLLNVAL) == 0 else { throw PacketSocketError.system(EBADF) }
                return
            }
            if ready == 0 { throw PacketSocketError.timeout }
            if errno != EINTR { throw PacketSocketError.system(errno) }
        }
    }

    private func readChunk(_ count: Int, until deadline: UInt64) throws -> Data {
        while true {
            try waitFor(Int16(POLLIN), until: deadline)
            var bytes = [UInt8](repeating: 0, count: count)
            let received = Darwin.read(descriptor, &bytes, count)
            if received >= 0 { return Data(bytes.prefix(received)) }
            if errno != EINTR, errno != EAGAIN { throw PacketSocketError.system(errno) }
        }
    }

    private func writeChunk(_ data: Data, until deadline: UInt64) throws -> Int {
        while true {
            try waitFor(Int16(POLLOUT), until: deadline)
            let written = data.withUnsafeBytes { Darwin.write(descriptor, $0.baseAddress, data.count) }
            if written >= 0 { return written }
            if errno != EINTR, errno != EAGAIN { throw PacketSocketError.system(errno) }
        }
    }
}
