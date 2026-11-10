import Foundation
import Darwin

// This probe is not linked into the installed provider or daemon.
enum BridgeIdentity {
    static let broker = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.client.bridge\""
    static let provider = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.client.tunnel\""
}

@objc protocol DescriptorProtocol {
    func hello(_ nonce: String, reply: @escaping (String) -> Void)
    func offer(_ handle: FileHandle, generation: String, reply: @escaping (Bool) -> Void)
}

private final class DescriptorSession: NSObject, DescriptorProtocol {
    private let lock = NSLock()
    private var greeted = false
    private var handle: FileHandle?
    private let onOffer: (FileHandle, String) -> Void

    init(onOffer: @escaping (FileHandle, String) -> Void) { self.onOffer = onOffer }

    func hello(_ nonce: String, reply: @escaping (String) -> Void) {
        guard UUID(uuidString: nonce) != nil else { reply(""); return }
        lock.lock()
        greeted = true
        lock.unlock()
        reply(nonce)
    }

    func offer(_ file: FileHandle, generation: String, reply: @escaping (Bool) -> Void) {
        var info = stat()
        guard UUID(uuidString: generation) != nil,
              fstat(file.fileDescriptor, &info) == 0,
              info.st_mode & mode_t(S_IFMT) == mode_t(S_IFSOCK) else {
            try? file.close()
            reply(false)
            return
        }
        lock.lock()
        guard greeted, handle == nil else {
            lock.unlock()
            try? file.close()
            reply(false)
            return
        }
        handle = file
        lock.unlock()
        onOffer(file, generation)
        reply(true)
    }

    func close() {
        lock.lock()
        let file = handle
        handle = nil
        greeted = false
        lock.unlock()
        if let file {
            shutdown(file.fileDescriptor, SHUT_RDWR)
            try? file.close()
        }
    }
}

@available(macOS 13.0, *)
final class DescriptorListener: NSObject, NSXPCListenerDelegate {
    private let listener = NSXPCListener.anonymous()
    private let requirement: String
    private let handler: (FileHandle, String) -> Void
    private let lock = NSLock()
    private var connections: [ObjectIdentifier: NSXPCConnection] = [:]
    var endpoint: NSXPCListenerEndpoint { listener.endpoint }

    init(peerRequirement: String, onOffer: @escaping (FileHandle, String) -> Void) {
        requirement = peerRequirement
        handler = onOffer
        super.init()
        listener.delegate = self
        listener.resume()
    }

    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection connection: NSXPCConnection) -> Bool {
        let session = DescriptorSession(onOffer: handler)
        let key = ObjectIdentifier(connection)
        connection.setCodeSigningRequirement(requirement)
        connection.exportedInterface = NSXPCInterface(with: DescriptorProtocol.self)
        connection.exportedObject = session
        connection.invalidationHandler = { [weak self] in
            session.close()
            guard let self else { return }
            self.lock.lock()
            self.connections.removeValue(forKey: key)
            self.lock.unlock()
        }
        lock.lock()
        connections[key] = connection
        lock.unlock()
        connection.resume()
        return true
    }

    func invalidate() {
        listener.invalidate()
        lock.lock()
        let current = Array(connections.values)
        connections.removeAll()
        lock.unlock()
        current.forEach { $0.invalidate() }
    }

    deinit { invalidate() }
}

private final class OfferCompletion {
    private let lock = NSLock()
    private var callback: ((Bool) -> Void)?
    init(_ callback: @escaping (Bool) -> Void) { self.callback = callback }
    func finish(_ accepted: Bool) {
        lock.lock()
        let callback = self.callback
        self.callback = nil
        lock.unlock()
        callback?(accepted)
    }
}

@available(macOS 13.0, *)
final class DescriptorClient {
    private let connection: NSXPCConnection
    init(endpoint: NSXPCListenerEndpoint, peerRequirement: String) {
        connection = NSXPCConnection(listenerEndpoint: endpoint)
        connection.remoteObjectInterface = NSXPCInterface(with: DescriptorProtocol.self)
        connection.setCodeSigningRequirement(peerRequirement)
        connection.resume()
    }

    func offer(_ handle: FileHandle, generation: String, reply: @escaping (Bool) -> Void) {
        let completion = OfferCompletion(reply)
        guard let proxy = connection.remoteObjectProxyWithErrorHandler({ _ in
            completion.finish(false)
        }) as? DescriptorProtocol else { completion.finish(false); return }
        let nonce = UUID().uuidString
        // The first verified incoming reply authenticates the server before any FD leaves.
        proxy.hello(nonce) { response in
            guard response == nonce else { completion.finish(false); return }
            proxy.offer(handle, generation: generation) { completion.finish($0) }
        }
    }

    func invalidate() { connection.invalidate() }
    deinit { invalidate() }
}
