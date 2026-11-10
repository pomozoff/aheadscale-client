import XCTest
import Foundation
import Darwin
@testable import AheadscaleBridgeProbe

final class NativeEngineControlTests: XCTestCase {
    func testInheritedControlRelaysAuthoritativeConfigurationSettingsAndReadiness() throws {
        var fds:[Int32]=[-1,-1];XCTAssertEqual(socketpair(AF_UNIX,SOCK_DGRAM,0,&fds),0)
        let control=FileHandle(fileDescriptor:fds[0],closeOnDealloc:true)
        let engine=FileHandle(fileDescriptor:fds[1],closeOnDealloc:true)
        defer{try? engine.close()}
        let triggered=expectation(description:"engine start requested")
        let stopped=expectation(description:"engine stop requested")
        let bridge=try NativeEngineControl(control:control,startEngine:{triggered.fulfill()},stopEngine:{stopped.fulfill()})
        let handlers=bridge.handlers
        let prepared=expectation(description:"configuration delivered")
        let generation="d75aef4f-7e19-4ae4-a841-6c250fc775c2"
        let data=try JSONSerialization.data(withJSONObject:["generation":generation,"version":1,"address":"100.88.1.2/16","mtu":1280,"dnsServers":["100.88.255.254"],"dnsDomains":["aheadscale.internal"]])
        handlers.prepare{received in XCTAssertEqual(received,data);prepared.fulfill()}
        wait(for:[triggered],timeout:2)
        XCTAssertEqual(data.withUnsafeBytes{Darwin.send(engine.fileDescriptor,$0.baseAddress,data.count,0)},data.count)
        wait(for:[prepared],timeout:2)
        let acknowledged=expectation(description:"settings sent")
        handlers.settings(generation,"utun42"){accepted in XCTAssertTrue(accepted);acknowledged.fulfill()}
        wait(for:[acknowledged],timeout:2)
        var buffer=[UInt8](repeating:0,count:4097)
        var p=pollfd(fd:engine.fileDescriptor,events:Int16(POLLIN),revents:0)
        XCTAssertEqual(poll(&p,1,2000),1)
        let count=recv(engine.fileDescriptor,&buffer,buffer.count,0)
        let ack=try JSONSerialization.jsonObject(with:Data(buffer.prefix(max(0,count)))) as? [String:String]
        XCTAssertEqual(ack?["kind"],"settings");XCTAssertEqual(ack?["interface"],"utun42")
        let ready=expectation(description:"engine ready delivered")
        handlers.ready(generation){accepted in XCTAssertTrue(accepted);ready.fulfill()}
        let event=try JSONSerialization.data(withJSONObject:["kind":"engine","generation":generation])
        _=event.withUnsafeBytes{Darwin.send(engine.fileDescriptor,$0.baseAddress,event.count,0)}
        wait(for:[ready],timeout:2)
        handlers.stop();wait(for:[stopped],timeout:2);bridge.close()
    }
}
