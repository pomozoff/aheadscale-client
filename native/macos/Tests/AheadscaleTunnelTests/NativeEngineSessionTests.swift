import XCTest
import Foundation
import NetworkExtension
import AheadscaleBridgeProbe
@testable import AheadscaleTunnel

private final class SessionRemote: NativeSessionRemote {
    let config: Data
    var settingsAcknowledged=false
    var engineReply: ((Bool)->Void)?
    var peer: PacketSocket?
    var onReadyRequest:(()->Void)?
    var onInvalidated:(()->Void)?
    init() throws {
        config=try JSONSerialization.data(withJSONObject:["version":1,"generation":"d75aef4f-7e19-4ae4-a841-6c250fc775c2","address":"100.88.1.2/16","mtu":1280,"dnsServers":["100.88.255.254"],"dnsDomains":["aheadscale.internal"]])
    }
    func configuration(_ reply:@escaping(Data)->Void){reply(config)}
    func offer(_ handle:FileHandle,generation:String,reply:@escaping(Bool)->Void){do{peer=try PacketSocket(duplicating:handle.fileDescriptor);reply(true)}catch{reply(false)}}
    func settings(generation:String,interface:String,reply:@escaping(Bool)->Void){settingsAcknowledged=true;reply(interface=="utun42")}
    func ready(generation:String,reply:@escaping(Bool)->Void){engineReply=reply;onReadyRequest?()}
    func invalidate(){peer?.shutdown();onInvalidated?()}
}
final class NativeEngineSessionTests: XCTestCase {
    func testProviderDoesNotReportConnectedBeforeEngineAcknowledges() throws {
        let remote=try SessionRemote();let requested=expectation(description:"engine readiness requested")
        let connected=expectation(description:"connected");let flow=TestPacketFlow()
        remote.onReadyRequest={requested.fulfill()}
        let session=NativeEngineSession(flow:flow,remote:remote,applySettings:{ config,reply in
            XCTAssertEqual(config.address,"100.88.1.2/16");reply("utun42",nil)
        },completion:{ error in XCTAssertNil(error);connected.fulfill() },onFailure:{_ in XCTFail("unexpected failure")})
        session.start();wait(for:[requested],timeout:2)
        XCTAssertTrue(remote.settingsAcknowledged)
        // No success was delivered by channel/settings alone.
        XCTAssertEqual(session.isReady,false)
        remote.engineReply?(true)
        wait(for:[connected],timeout:2);XCTAssertTrue(session.isReady)
        let stopped=expectation(description:"stopped");session.stop{stopped.fulfill()};wait(for:[stopped],timeout:2)
    }
    func testCancelBeforeSettingsIgnoresLateCompletionAndNeverStartsEngine() throws {
        let remote=try SessionRemote();let applied=expectation(description:"settings pending")
        let cancelled=expectation(description:"start cancelled");let stopped=expectation(description:"stopped")
        var settingsReply:((String?,Error?)->Void)?
        let session=NativeEngineSession(flow:TestPacketFlow(),remote:remote,applySettings:{_,reply in settingsReply=reply;applied.fulfill()},completion:{error in XCTAssertNotNil(error);cancelled.fulfill()},onFailure:{_ in XCTFail("user cancel must not show failure")})
        session.start();wait(for:[applied],timeout:2)
        session.stop{stopped.fulfill()};wait(for:[cancelled,stopped],timeout:2)
        settingsReply?("utun42",nil)
        XCTAssertFalse(session.isReady)
        XCTAssertFalse(remote.settingsAcknowledged)
    }
    func testSelfIPv4ReturnsToOSWithoutEnteringWireGuard() throws {
        for proto: UInt8 in [1, 6] {
            let remote = try SessionRemote()
            let flow = TestPacketFlow()
            let read = expectation(description: "OS read")
            let reflected = expectation(description: "self packet returned to OS")
            let ready = expectation(description: "engine requested")
            flow.requested = read
            flow.received = reflected
            remote.onReadyRequest = { ready.fulfill() }
            let session = NativeEngineSession(flow: flow, remote: remote,
                applySettings: { _, reply in reply("utun42", nil) },
                completion: { _ in }, onFailure: { _ in XCTFail("unexpected failure") })
            session.start()
            wait(for: [read, ready], timeout: 2)
            remote.engineReply?(true)
            var packet = ProbeScenario.packet(sequence: 2)
            packet[9] = proto
            packet.replaceSubrange(12..<20, with: [100,88,1,2,100,88,1,2])
            flow.deliver([packet], families: [NSNumber(value: AF_INET)])
            wait(for: [reflected], timeout: 2)
            XCTAssertEqual(flow.packets, [packet])
            XCTAssertFalse(try XCTUnwrap(remote.peer).waitForReadable(timeoutMilliseconds: 50))
            let stopped = expectation(description: "stopped")
            session.stop { stopped.fulfill() }
            wait(for: [stopped], timeout: 2)
        }
    }

}
