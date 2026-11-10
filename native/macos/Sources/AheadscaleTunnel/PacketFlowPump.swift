import Foundation
import NetworkExtension
import Darwin
import AheadscaleBridgeProbe

// The OS boundary is substituted in tests; the descriptor transport is real.
protocol PacketFlowIO: AnyObject {
    func readPackets(completionHandler: @escaping @Sendable ([Data], [NSNumber]) -> Void)
    func writePackets(_ packets: [Data], withProtocols protocols: [NSNumber]) -> Bool
}
extension NEPacketTunnelFlow: PacketFlowIO {}

enum PacketFlowPumpError: Error {
    case invalidMTU, invalidBatch, invalidPacket, invalidFamily, flowRefused
}

// One read worker and one write worker, with only one outstanding OS read/batch.
// No network settings or authentication are performed here. The caller supplies
// an authenticated channel and starts this only after applying session settings.
// Mutable state is locked; each IO direction has exactly one worker. The flow
// implementation must support one reader and one writer, as NEPacketTunnelFlow does.
final class PacketFlowPump: @unchecked Sendable {
    private enum State { case idle, running, finished }
    private let lock = NSLock()
    // Serializes synchronous OS delivery with stop; recursive for re-entrant stop.
    private let deliveryLock = NSRecursiveLock()
    private var state: State = .idle
    private let flow: PacketFlowIO
    private let socket: PacketSocket
    private let mtu: Int
    private let localIPv4: Data?
    private let onStop: (Error?) -> Void
    private let outbound = DispatchQueue(label: "ru.aheadscale.packetflow.outbound")
    private let inbound = DispatchQueue(label: "ru.aheadscale.packetflow.inbound")

    init(flow: PacketFlowIO, socket: PacketSocket, mtu: Int, localIPv4: Data? = nil, onStop: @escaping (Error?) -> Void) {
        self.flow = flow
        self.socket = socket
        self.mtu = mtu
        self.localIPv4 = localIPv4
        self.onStop = onStop
    }

    func start() {
        lock.lock()
        guard state == .idle else { lock.unlock(); return }
        state = .running
        lock.unlock()
        guard (1...65535).contains(mtu) else { finish(PacketFlowPumpError.invalidMTU); return }
        requestPackets()
        inbound.async { [weak self] in self?.receivePackets() }
    }

    func stop() { finish(nil) }

    private var running: Bool {
        lock.lock()
        defer { lock.unlock() }
        return state == .running
    }

    private func requestPackets() {
        guard running else { return }
        flow.readPackets { [weak self] packets, families in
            guard let self, self.running else { return }
            self.outbound.async { [weak self] in
                guard let self, self.running else { return }
                do {
                    guard !packets.isEmpty, packets.count == families.count else {
                        throw PacketFlowPumpError.invalidBatch
                    }
                    for (packet, family) in zip(packets, families) {
                        guard self.running else { return }
                        guard try Self.family(packet, mtu: self.mtu) == family.intValue else {
                            throw PacketFlowPumpError.invalidFamily
                        }
                        if family.intValue == Int(AF_INET), let address = self.localIPv4,
                           address.count == 4,
                           packet.subdata(in: 12..<16) == address,
                           packet.subdata(in: 16..<20) == address {
                            // Outbound packets to this session's own address belong
                            // to the local OS, never to a remote WireGuard peer.
                            try self.deliver(packet, family: Int(AF_INET))
                        } else {
                            try self.socket.writeFrame(packet, mtu: self.mtu, timeoutMilliseconds: 2000)
                        }
                    }
                    self.requestPackets()
                } catch { self.finish(error) }
            }
        }
    }

    private func receivePackets() {
        do {
            while running {
                // Silence is normal. Start the frame deadline only on readability.
                guard try socket.waitForReadable(timeoutMilliseconds: 1000) else { continue }
                guard running else { return }
                let packet = try socket.readFrame(mtu: mtu, timeoutMilliseconds: 2000)
                let family = try Self.family(packet, mtu: mtu)
                try deliver(packet, family: family)
            }
        } catch { finish(error) }
    }

    private func deliver(_ packet: Data, family: Int) throws {
        deliveryLock.lock()
        defer { deliveryLock.unlock() }
        guard running else { return }
        guard flow.writePackets([packet], withProtocols: [NSNumber(value: family)]) else {
            throw PacketFlowPumpError.flowRefused
        }
    }

    private func finish(_ error: Error?) {
        deliveryLock.lock()
        lock.lock()
        guard state != .finished else { lock.unlock(); deliveryLock.unlock(); return }
        state = .finished
        lock.unlock()
        socket.shutdown() // Unblocks IO without acquiring either directional lock.
        deliveryLock.unlock()
        onStop(error)
    }

    private static func family(_ packet: Data, mtu: Int) throws -> Int {
        guard !packet.isEmpty, packet.count <= mtu else { throw PacketFlowPumpError.invalidPacket }
        let bytes = [UInt8](packet.prefix(40))
        switch bytes[0] >> 4 {
        case 4:
            guard packet.count >= 20 else { throw PacketFlowPumpError.invalidPacket }
            let headerLength = Int(bytes[0] & 15) * 4
            let length = Int(bytes[2]) << 8 | Int(bytes[3])
            guard headerLength >= 20, headerLength <= packet.count, length == packet.count else {
                throw PacketFlowPumpError.invalidPacket
            }
            return Int(AF_INET)
        case 6:
            guard packet.count >= 40,
                  (Int(bytes[4]) << 8 | Int(bytes[5])) + 40 == packet.count else {
                throw PacketFlowPumpError.invalidPacket
            }
            return Int(AF_INET6)
        default: throw PacketFlowPumpError.invalidPacket
        }
    }
}
