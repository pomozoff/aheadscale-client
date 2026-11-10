import Foundation
import Darwin
import AheadscaleBridgeProbe

guard geteuid() == 0, CommandLine.arguments.count == 1 else {
    fputs("Только временный root fixture в macOS-ВМ.\n", stderr)
    exit(1)
}
@available(macOS 13.0, *)
func runRootProof() -> Never {
    let listener = DescriptorListener(machServiceName: RootBridgeProofIdentity.service,
                                      peerRequirement: RootBridgeProofIdentity.providerRequirement) { handle, _ in
        let socket = try PacketSocket(duplicating: handle.fileDescriptor)
        NSLog("Aheadscale root bridge proof: authenticated descriptor received")
        DispatchQueue.global().async {
            defer { socket.shutdown() }
            do {
                for _ in 0..<2 {
                    let packet = try socket.readFrame(mtu: 1280, timeoutMilliseconds: 2000)
                    try socket.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 2000)
                }
                NSLog("Aheadscale root bridge proof: two packets echoed")
            } catch { NSLog("Aheadscale root bridge proof: channel closed") }
        }
    }
    NSLog("Aheadscale root bridge proof: ready")
    DispatchQueue.global().asyncAfter(deadline: .now() + 180) {
        listener.invalidate()
        exit(0)
    }
    dispatchMain()
}

if #available(macOS 13.0, *) { runRootProof() }
else { exit(1) }
