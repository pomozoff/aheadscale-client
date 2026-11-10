import Foundation
import Darwin
import AheadscaleBridgeProbe

protocol ProviderProofSession: AnyObject {
    func start()
    func stop()
}

enum ProviderBridgeProofError: Error { case unavailable, cancelled, timedOut, mismatchedPacket }

// Native-only prerequisite. Even success is not engine readiness and cannot
// complete startTunnel successfully. Uses only synthetic packets, no OS routes.
@available(macOS 13.0, *)
final class ProviderBridgeProof: ProviderProofSession {
    private let lock = NSLock()
    private var started = false
    private var completion: ((Result<Void, Error>) -> Void)?
    private var client: DescriptorClient?
    private var socket: PacketSocket?
    private var remote: FileHandle?

    init(completion: @escaping (Result<Void, Error>) -> Void) { self.completion = completion }

    func start() {
        lock.lock()
        guard !started, completion != nil else { lock.unlock(); return }
        started = true
        lock.unlock()
        var descriptors: [Int32] = [-1, -1]
        guard socketpair(AF_UNIX, SOCK_STREAM, 0, &descriptors) == 0 else {
            finish(.failure(ProviderBridgeProofError.unavailable)); return
        }
        let local: PacketSocket
        do { local = try PacketSocket(duplicating: descriptors[0]) }
        catch {
            Darwin.close(descriptors[0]); Darwin.close(descriptors[1])
            finish(.failure(error)); return
        }
        Darwin.close(descriptors[0])
        let offered = FileHandle(fileDescriptor: descriptors[1], closeOnDealloc: true)
        let connection = DescriptorClient(systemMachServiceName: RootBridgeProofIdentity.service,
                                          peerRequirement: RootBridgeProofIdentity.brokerRequirement)
        lock.lock()
        guard completion != nil else {
            lock.unlock(); local.shutdown(); connection.invalidate(); return
        }
        client = connection
        socket = local
        remote = offered
        lock.unlock()
        DispatchQueue.global().asyncAfter(deadline: .now() + 5) { [weak self] in
            self?.finish(.failure(ProviderBridgeProofError.timedOut))
        }
        connection.offer(offered, generation: UUID().uuidString) { [weak self] accepted in
            guard let self else { return }
            guard accepted else { self.finish(.failure(ProviderBridgeProofError.unavailable)); return }
            DispatchQueue.global().async { [weak self] in
                guard let self else { return }
                do {
                    for sequence: UInt64 in 0..<2 {
                        let packet = ProbeScenario.packet(sequence: sequence)
                        try local.writeFrame(packet, mtu: 1280, timeoutMilliseconds: 1000)
                        guard try local.readFrame(mtu: 1280, timeoutMilliseconds: 1000) == packet else {
                            throw ProviderBridgeProofError.mismatchedPacket
                        }
                    }
                    self.finish(.success(()))
                } catch { self.finish(.failure(error)) }
            }
        }
    }

    func stop() { finish(.failure(ProviderBridgeProofError.cancelled)) }

    private func finish(_ result: Result<Void, Error>) {
        lock.lock()
        guard let callback = completion else { lock.unlock(); return }
        completion = nil
        let client = self.client
        let socket = self.socket
        let remote = self.remote
        self.client = nil; self.socket = nil; self.remote = nil
        lock.unlock()
        socket?.shutdown()
        client?.invalidate()
        if let remote { try? remote.close() }
        callback(result)
    }
}
