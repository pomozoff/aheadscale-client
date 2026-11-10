import Foundation
import Darwin
import AheadscaleBridgeProbe

enum ProbeFailure: Error {
    case unsupportedOS, handshakeTimeout, refused, unexpectedAcceptance, mismatchedPacket, selfCheck
}

func report(_ fields: [String: Any]) {
    do {
        let data = try JSONSerialization.data(withJSONObject: fields, options: [.sortedKeys])
        print(String(decoding: data, as: UTF8.self))
        fflush(stdout)
    } catch {
        fputs("probe report serialization failed\n", stderr)
        exit(2)
    }
}

@available(macOS 13.0, *)
func serve(_ options: ProbeScenario) -> Never {
    let listener = DescriptorListener(machServiceName: ProbeScenario.serviceName,
                                      peerRequirement: ProbeScenario.clientRequirement) { handle, _ in
        let socket = try PacketSocket(duplicating: handle.fileDescriptor)
        report(["event": "descriptor_received"])
        DispatchQueue.global().async {
            defer { socket.shutdown() }
            do {
                while true {
                    let packet = try socket.readFrame(mtu: 1280, timeoutMilliseconds: 5000)
                    try socket.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 5000)
                }
            } catch { report(["event": "channel_closed", "reason": String(describing: error)]) }
        }
    }
    report(["event": "server_started", "service": ProbeScenario.serviceName])
    DispatchQueue.global().asyncAfter(deadline: .now() + .seconds(options.seconds)) {
        listener.invalidate()
        report(["event": "server_deadline"])
        exit(0)
    }
    dispatchMain()
}

@available(macOS 13.0, *)
func runClient(_ options: ProbeScenario) throws {
    var descriptors: [Int32] = [-1, -1]
    guard socketpair(AF_UNIX, SOCK_STREAM, 0, &descriptors) == 0 else {
        throw PacketSocketError.system(errno)
    }
    defer { Darwin.close(descriptors[0]); Darwin.close(descriptors[1]) }
    let socket = try PacketSocket(duplicating: descriptors[0])
    defer { socket.shutdown() }
    let remote = FileHandle(fileDescriptor: descriptors[1], closeOnDealloc: false)
    let client = DescriptorClient(machServiceName: ProbeScenario.serviceName,
                                  peerRequirement: ProbeScenario.serverRequirement)
    defer { client.invalidate() }
    let completed = DispatchSemaphore(value: 0)
    var accepted = false
    client.offer(remote, generation: UUID().uuidString) { result in
        accepted = result
        completed.signal()
    }
    guard completed.wait(timeout: .now() + 10) == .success else { throw ProbeFailure.handshakeTimeout }
    if options.expectRefusal {
        guard !accepted else { throw ProbeFailure.unexpectedAcceptance }
        report(["event": "refused"])
        return
    }
    guard accepted else { throw ProbeFailure.refused }
    let start = DispatchTime.now().uptimeNanoseconds
    let end = start + UInt64(options.seconds) * 1_000_000_000
    var sequence: UInt64 = 0
    var ipv4 = 0
    var ipv6 = 0
    while DispatchTime.now().uptimeNanoseconds < end {
        let packet = ProbeScenario.packet(sequence: sequence)
        try socket.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 2000)
        let echoed = try socket.readFrame(mtu: 1280, timeoutMilliseconds: 2000)
        guard echoed == packet else { throw ProbeFailure.mismatchedPacket }
        if packet[0] >> 4 == 4 { ipv4 += 1 } else { ipv6 += 1 }
        sequence += 1
        usleep(10_000)
    }
    let elapsed = Double(DispatchTime.now().uptimeNanoseconds - start) / 1_000_000_000
    report(["event": "pass", "frames": sequence, "ipv4": ipv4, "ipv6": ipv6, "seconds": elapsed])
}

do {
    let arguments = Array(CommandLine.arguments.dropFirst())
    if arguments == ["--self-check"] {
        guard ProbeScenario.packet(sequence: 0)[0] >> 4 == 4,
              ProbeScenario.packet(sequence: 1)[0] >> 4 == 6 else { throw ProbeFailure.selfCheck }
        report(["event": "self_check_pass"])
        exit(0)
    }
    let options = try ProbeScenario.parse(arguments)
    guard #available(macOS 13.0, *) else { throw ProbeFailure.unsupportedOS }
    switch options.role {
    case .server: serve(options)
    case .client: try runClient(options)
    }
} catch {
    report(["event": "failed", "reason": String(describing: error)])
    exit(1)
}
