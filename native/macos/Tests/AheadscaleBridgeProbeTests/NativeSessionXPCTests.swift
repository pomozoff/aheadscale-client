import XCTest
import Foundation
import Security
import Darwin
@testable import AheadscaleBridgeProbe

final class NativeSessionXPCTests: XCTestCase {
    private func requirement() throws -> String {
        var code: SecCode?; var staticCode: SecStaticCode?; var req: SecRequirement?; var text: CFString?
        XCTAssertEqual(SecCodeCopySelf([], &code), errSecSuccess)
        XCTAssertEqual(SecCodeCopyStaticCode(try XCTUnwrap(code), [], &staticCode), errSecSuccess)
        XCTAssertEqual(SecCodeCopyDesignatedRequirement(try XCTUnwrap(staticCode), [], &req), errSecSuccess)
        XCTAssertEqual(SecRequirementCopyString(try XCTUnwrap(req), [], &text), errSecSuccess)
        return try XCTUnwrap(text) as String
    }
    func testConfigurationAndReadinessRequireVerifiedServerAndCurrentGeneration() throws {
        guard #available(macOS 13.0, *) else { throw XCTSkip("macOS 13") }
        let generation = UUID().uuidString.lowercased()
        let data = try JSONSerialization.data(withJSONObject: ["generation": generation])
        let prepared = expectation(description: "prepare")
        let settings = expectation(description: "settings")
        let ready = expectation(description: "engine ready")
        let listener = NativeSessionListener(peerRequirement: try requirement(), handlers: .init(
            prepare: { reply in prepared.fulfill(); reply(data) },
            offer: { _, g in XCTAssertEqual(g, generation) },
            settings: { g, name, reply in XCTAssertEqual(g, generation); XCTAssertEqual(name,"utun42"); settings.fulfill(); reply(true) },
            ready: { g, reply in XCTAssertEqual(g,generation); ready.fulfill(); reply(true) },
            stop: {}))
        defer { listener.invalidate() }
        let client = NativeSessionClient(endpoint: listener.endpoint, peerRequirement: try requirement())
        defer { client.invalidate() }
        let complete = expectation(description: "round trip")
        client.configuration { config in
            XCTAssertEqual(config,data)
            var fds: [Int32] = [-1,-1]
            guard socketpair(AF_UNIX, SOCK_STREAM,0,&fds)==0 else { XCTFail("socketpair"); complete.fulfill(); return }
            let a=FileHandle(fileDescriptor:fds[0],closeOnDealloc:true)
            let b=FileHandle(fileDescriptor:fds[1],closeOnDealloc:true)
            client.offer(a,generation:generation) { accepted in
                XCTAssertTrue(accepted)
                client.settings(generation:generation,interface:"utun42") { accepted in
                    XCTAssertTrue(accepted)
                    client.ready(generation:generation) { accepted in
                        XCTAssertTrue(accepted); try? b.close(); complete.fulfill()
                    }
                }
            }
        }
        wait(for:[prepared,settings,ready,complete],timeout:5)
    }
    func testForeignProviderCannotStartRootEngine() throws {
        guard #available(macOS 13.0, *) else { throw XCTSkip("macOS 13") }
        let forbidden=expectation(description:"foreign provider never prepares engine"); forbidden.isInverted=true
        let listener=NativeSessionListener(peerRequirement:BridgeIdentity.provider, handlers:.init(
            prepare:{ reply in forbidden.fulfill();reply(Data()) },offer:{_,_ in},settings:{_,_,r in r(false)},ready:{_,r in r(false)},stop:{}))
        defer{listener.invalidate()}
        let client=NativeSessionClient(endpoint:listener.endpoint,peerRequirement:try requirement())
        defer{client.invalidate()}
        let refused=expectation(description:"refused")
        client.configuration{ data in XCTAssertTrue(data.isEmpty);refused.fulfill() }
        wait(for:[refused,forbidden],timeout:5)
    }
}
