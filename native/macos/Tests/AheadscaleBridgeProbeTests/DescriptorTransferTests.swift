import XCTest
import Foundation
import Security
import Darwin
@testable import AheadscaleBridgeProbe

final class DescriptorTransferTests: XCTestCase {
    private func ownRequirement() throws -> String {
        var code: SecCode?
        XCTAssertEqual(SecCodeCopySelf([], &code), errSecSuccess)
        var staticCode: SecStaticCode?
        XCTAssertEqual(SecCodeCopyStaticCode(try XCTUnwrap(code), [], &staticCode), errSecSuccess)
        var requirement: SecRequirement?
        XCTAssertEqual(SecCodeCopyDesignatedRequirement(try XCTUnwrap(staticCode), [], &requirement), errSecSuccess)
        var text: CFString?
        XCTAssertEqual(SecRequirementCopyString(try XCTUnwrap(requirement), [], &text), errSecSuccess)
        return try XCTUnwrap(text) as String
    }

    private func sockets() throws -> (FileHandle, FileHandle) {
        var pair: [Int32] = [-1, -1]
        guard socketpair(AF_UNIX, SOCK_STREAM, 0, &pair) == 0 else {
            throw NSError(domain: NSPOSIXErrorDomain, code: Int(errno))
        }
        return (FileHandle(fileDescriptor: pair[0], closeOnDealloc: true),
                FileHandle(fileDescriptor: pair[1], closeOnDealloc: true))
    }

    func testAuthenticatedDescriptorCarriesBytes() throws {
        guard #available(macOS 13.0, *) else { throw XCTSkip("XPC signature enforcement requires macOS 13") }
        let requirement = try ownRequirement()
        let listener = DescriptorListener(peerRequirement: requirement) { handle, _ in
            handle.write(Data("pong".utf8))
        }
        defer { listener.invalidate() }
        let client = DescriptorClient(endpoint: listener.endpoint, peerRequirement: requirement)
        defer { client.invalidate() }
        let (local, remote) = try sockets()
        defer { try? local.close(); try? remote.close() }
        let offered = expectation(description: "descriptor accepted")
        client.offer(remote, generation: UUID().uuidString) { accepted in
            XCTAssertTrue(accepted, "Verified peers must accept the descriptor")
            offered.fulfill()
        }
        wait(for: [offered], timeout: 5)
        var item = pollfd(fd: local.fileDescriptor, events: Int16(POLLIN), revents: 0)
        XCTAssertEqual(poll(&item, 1, 2000), 1, "Transferred descriptor must deliver bytes")
        if item.revents & Int16(POLLIN) != 0 {
            XCTAssertEqual(local.readData(ofLength: 4), Data("pong".utf8))
        }
    }

    func testForeignClientCannotDeliverDescriptor() throws {
        guard #available(macOS 13.0, *) else { throw XCTSkip("macOS 13 required") }
        try refusal(listenerRequirement: BridgeIdentity.broker, serverRequirement: ownRequirement())
    }

    func testForeignServerCannotReceiveDescriptorBeforeHandshake() throws {
        guard #available(macOS 13.0, *) else { throw XCTSkip("macOS 13 required") }
        try refusal(listenerRequirement: ownRequirement(), serverRequirement: BridgeIdentity.provider)
    }

    @available(macOS 13.0, *)
    private func refusal(listenerRequirement: String, serverRequirement: String) throws {
        let forbidden = expectation(description: "no descriptor crosses unverified peer")
        forbidden.isInverted = true
        let listener = DescriptorListener(peerRequirement: listenerRequirement) { _, _ in forbidden.fulfill() }
        defer { listener.invalidate() }
        let client = DescriptorClient(endpoint: listener.endpoint, peerRequirement: serverRequirement)
        defer { client.invalidate() }
        let (local, remote) = try sockets()
        defer { try? local.close(); try? remote.close() }
        let refused = expectation(description: "signature refusal surfaced")
        client.offer(remote, generation: UUID().uuidString) { accepted in
            XCTAssertFalse(accepted, "Unverified peer must be refused")
            refused.fulfill()
        }
        wait(for: [refused, forbidden], timeout: 5)
    }

    func testInvalidGenerationIsRefused() throws {
        guard #available(macOS 13.0, *) else { throw XCTSkip("macOS 13 required") }
        let requirement = try ownRequirement()
        let forbidden = expectation(description: "invalid generation not offered")
        forbidden.isInverted = true
        let listener = DescriptorListener(peerRequirement: requirement) { _, _ in forbidden.fulfill() }
        defer { listener.invalidate() }
        let client = DescriptorClient(endpoint: listener.endpoint, peerRequirement: requirement)
        defer { client.invalidate() }
        let (local, remote) = try sockets()
        defer { try? local.close(); try? remote.close() }
        let refused = expectation(description: "generation refused")
        client.offer(remote, generation: "old-invalid") { accepted in
            XCTAssertFalse(accepted, "Malformed generation must not be accepted")
            refused.fulfill()
        }
        wait(for: [refused, forbidden], timeout: 5)
    }
}
