import Foundation
import Darwin

// One private, locked inode coordinates independently launched GUI/CLI helpers.
// No lock is held across an await. Rechecking the token and starting/stopping
// the tunnel happen inside the same short critical section.
public final class VPNControlLease {
    private let descriptor: Int32
    public init(url: URL) throws {
        var parent = stat()
        guard lstat(url.deletingLastPathComponent().path, &parent) == 0,
              parent.st_mode & mode_t(S_IFMT) == mode_t(S_IFDIR),
              parent.st_uid == geteuid(), parent.st_mode & 0o077 == 0 else { throw Self.error() }
        let fd = open(url.path, O_CREAT | O_RDWR | O_CLOEXEC | O_NOFOLLOW, 0o600)
        guard fd >= 0 else { throw Self.error() }
        var info = stat()
        guard fstat(fd, &info) == 0, info.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG),
              info.st_uid == geteuid(), info.st_mode & 0o077 == 0, info.st_nlink == 1 else {
            Darwin.close(fd); throw Self.error()
        }
        descriptor = fd
    }
    deinit { Darwin.close(descriptor) }
    public static func userLease() throws -> VPNControlLease {
        guard geteuid() != 0 else { throw error() }
        let directory = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/AheadscaleNativeControl", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700])
        return try VPNControlLease(url: directory.appendingPathComponent("generation"))
    }
    public func claim(_ token: UUID) throws {
        try locked {
            let bytes = Array(token.uuidString.lowercased().utf8)
            guard lseek(descriptor, 0, SEEK_SET) == 0, ftruncate(descriptor, 0) == 0,
                  bytes.withUnsafeBytes({ Darwin.write(descriptor, $0.baseAddress, $0.count) }) == bytes.count,
                  fsync(descriptor) == 0 else { throw Self.error() }
        }
    }
    public func isCurrent(_ token: UUID) throws -> Bool {
        try locked { try matches(token) }
    }
    public func withCurrent(_ token: UUID, _ body: () throws -> Void) throws -> Bool {
        try locked {
            guard try matches(token) else { return false }
            try body(); return true
        }
    }
    private func matches(_ token: UUID) throws -> Bool {
        var bytes = [UInt8](repeating: 0, count: 64)
        guard lseek(descriptor, 0, SEEK_SET) == 0 else { throw Self.error() }
        let count = bytes.withUnsafeMutableBytes { Darwin.read(descriptor, $0.baseAddress, $0.count) }
        guard count >= 0 else { throw Self.error() }
        return String(bytes: bytes.prefix(count), encoding: .utf8) == token.uuidString.lowercased()
    }
    private func locked<T>(_ body: () throws -> T) throws -> T {
        let end = ProcessInfo.processInfo.systemUptime + 1
        while flock(descriptor, LOCK_EX | LOCK_NB) != 0 {
            guard errno == EWOULDBLOCK || errno == EINTR,
                  ProcessInfo.processInfo.systemUptime < end else { throw Self.error() }
            usleep(1000)
        }
        defer { flock(descriptor, LOCK_UN) }
        return try body()
    }
    private static func error() -> NSError { NSError(domain: "ru.aheadscale.control", code: 2) }
}
