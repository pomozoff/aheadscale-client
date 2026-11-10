import Foundation
import NetworkExtension
import Darwin
import AheadscaleBridgeProbe

// The OS boundary is substituted in tests; the descriptor transport is real.
protocol PacketFlowIO: AnyObject {
    func readPackets(completionHandler: @escaping ([Data], [NSNumber]) -> Void)
    func writePackets(_ packets: [Data], withProtocols protocols: [NSNumber]) -> Bool
}
extension NEPacketTunnelFlow: PacketFlowIO {}

enum PacketFlowPumpError: Error {
    case invalidMTU, invalidBatch, invalidPacket, invalidFamily, flowRefused
}

// One read worker and one write worker, with only one outstanding OS read/batch.
// No network settings or authentication are performed here. The caller supplies
// an authenticated channel and starts this only after applying session settings.
final class PacketFlowPump {
    private enum State { case idle, running, finished }
    private let lock = NSLock()
    // Serializes synchronous OS delivery with stop; recursive for re-entrant stop.
    private let deliveryLock = NSRecursiveLock()
    private var state: State = .idle
    private let flow: PacketFlowIO
    private let socket: PacketSocket
    private let mtu: Int
    private let onStop: (Error?) -> Void
    private let outbound = DispatchQueue(label: "ru.aheadscale.packetflow.outbound")
    private let inbound = DispatchQueue(label: "ru.aheadscale.packetflow.inbound")

    init(flow: PacketFlowIO, socket: PacketSocket, mtu: Int, onStop: @escaping (Error?) -> Void) {
        self.flow = flow
        self.socket = socket
        self.mtu = mtu
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
                        try self.socket.writeFrame(packet, mtu: self.mtu, timeoutMilliseconds: 2000)
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
                deliveryLock.lock()
                let accepted = !running || flow.writePackets([packet], withProtocols: [NSNumber(value: family)])
                deliveryLock.unlock()
                if !accepted { throw PacketFlowPumpError.flowRefused }
            }
        } catch { finish(error) }
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
