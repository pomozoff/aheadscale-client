import Foundation
import Darwin
import CDescriptorHandoff

public enum NativeDescriptorHandoffError: Error, Equatable {
    case invalidGeneration
    case system(Int32)
}

// Call only after XPC peer verification and binding to the authoritative root
// generation. The control endpoint must be inherited from the trusted parent,
// never accepted from a client. Neither borrowed descriptor is closed here.
public enum NativeDescriptorHandoff {
    public static func send(packetFD: Int32, generation: String, controlFD: Int32) throws {
        guard let id = UUID(uuidString: generation), id.uuidString.lowercased() == generation,
              id != UUID(uuid: (0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0)) else {
            throw NativeDescriptorHandoffError.invalidGeneration
        }
        var bytes = id.uuid
        let result = withUnsafeBytes(of: &bytes) { buffer in
            aheadscale_send_packet_fd(controlFD, packetFD, buffer.bindMemory(to: UInt8.self).baseAddress)
        }
        guard result == 0 else { throw NativeDescriptorHandoffError.system(errno) }
    }
}
