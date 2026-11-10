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
    private var closed = false
    private var handle: FileHandle?
    private let onOffer: (FileHandle, String) throws -> Void

    init(onOffer: @escaping (FileHandle, String) throws -> Void) { self.onOffer = onOffer }

    func hello(_ nonce: String, reply: @escaping (String) -> Void) {
        guard UUID(uuidString: nonce) != nil else { reply(""); return }
        lock.lock()
        guard !closed else { lock.unlock(); reply(""); return }
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
        guard !closed, greeted, handle == nil else {
            lock.unlock()
            try? file.close()
            reply(false)
            return
        }
        handle = file
        do {
            // Keep the borrowed descriptor alive until setup has duplicated it.
            // The callback must schedule IO elsewhere rather than block this lock.
            try onOffer(file, generation)
        } catch {
            handle = nil
            greeted = false
            closed = true
            lock.unlock()
            shutdown(file.fileDescriptor, SHUT_RDWR)
            try? file.close()
            reply(false)
            return
        }
        lock.unlock()
        reply(true)
    }

    func close() {
        lock.lock()
        let file = handle
        closed = true
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
public final class DescriptorListener: NSObject, NSXPCListenerDelegate {
    private let listener: NSXPCListener
    private let requirement: String
    private let handler: (FileHandle, String) throws -> Void
    private let lock = NSLock()
    private var connections: [ObjectIdentifier: NSXPCConnection] = [:]
    private var closed = false
    public var endpoint: NSXPCListenerEndpoint { listener.endpoint }

    public convenience init(peerRequirement: String, onOffer: @escaping (FileHandle, String) throws -> Void) {
        self.init(listener: .anonymous(), peerRequirement: peerRequirement, onOffer: onOffer)
    }

    public convenience init(machServiceName: String, peerRequirement: String,
                            onOffer: @escaping (FileHandle, String) throws -> Void) {
        self.init(listener: NSXPCListener(machServiceName: machServiceName),
                  peerRequirement: peerRequirement, onOffer: onOffer)
    }

    private init(listener: NSXPCListener, peerRequirement: String,
                 onOffer: @escaping (FileHandle, String) throws -> Void) {
        self.listener = listener
        requirement = peerRequirement
        handler = onOffer
        super.init()
        listener.delegate = self
        listener.resume()
    }

    public func listener(_ listener: NSXPCListener, shouldAcceptNewConnection connection: NSXPCConnection) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        guard !closed else { return false }
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
        connections[key] = connection
        connection.resume()
        return true
    }

    public func invalidate() {
        lock.lock()
        closed = true
        let current = Array(connections.values)
        connections.removeAll()
        lock.unlock()
        listener.invalidate()
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
public final class DescriptorClient {
    private let connection: NSXPCConnection
    public convenience init(endpoint: NSXPCListenerEndpoint, peerRequirement: String) {
        self.init(connection: NSXPCConnection(listenerEndpoint: endpoint), peerRequirement: peerRequirement)
    }

    public convenience init(machServiceName: String, peerRequirement: String) {
        self.init(connection: NSXPCConnection(machServiceName: machServiceName, options: []),
                  peerRequirement: peerRequirement)
    }

    private init(connection: NSXPCConnection, peerRequirement: String) {
        self.connection = connection
        connection.remoteObjectInterface = NSXPCInterface(with: DescriptorProtocol.self)
        connection.setCodeSigningRequirement(peerRequirement)
        connection.resume()
    }

    public func offer(_ handle: FileHandle, generation: String, reply: @escaping (Bool) -> Void) {
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

    public func invalidate() { connection.invalidate() }
    deinit { invalidate() }
}
