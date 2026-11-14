import XCTest
import Foundation
@testable import AheadscaleActivation

final class VPNControlTests: XCTestCase {
    func testStrictActionsRejectUnknownFlagsAndArguments() throws {
        for action in ["status", "connect", "disconnect", "cancel"] {
            XCTAssertEqual(try VPNControlAction.parse([action]).rawValue, action)
        }
        for args in [[], ["up"], ["status", "connect"], ["connect", "--server", "https://other"], ["--help"]] {
            XCTAssertThrowsError(try VPNControlAction.parse(args))
        }
    }

    func testNewProcessLeaseInvalidatesOldConnectBeforeMutation() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false,
            attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: directory) }
        let path = directory.appendingPathComponent("generation")
        let old = try VPNControlLease(url: path)
        let newer = try VPNControlLease(url: path)
        let connect = UUID(), cancel = UUID()
        try old.claim(connect)
        XCTAssertTrue(try old.isCurrent(connect))
        try newer.claim(cancel)
        var changes = 0
        XCTAssertFalse(try old.withCurrent(connect) { changes += 1 })
        XCTAssertTrue(try newer.withCurrent(cancel) { changes += 1 })
        XCTAssertEqual(changes, 1)
    }

    func testLeaseRejectsSymlinkAndPublicFile() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false,
            attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: directory) }
        let original = directory.appendingPathComponent("original")
        try Data("not-a-lease".utf8).write(to: original)
        let link = directory.appendingPathComponent("link")
        try FileManager.default.createSymbolicLink(at: link, withDestinationURL: original)
        XCTAssertThrowsError(try VPNControlLease(url: link))
        try FileManager.default.setAttributes([.posixPermissions: 0o644], ofItemAtPath: original.path)
        XCTAssertThrowsError(try VPNControlLease(url: original))
    }
}
