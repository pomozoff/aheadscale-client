import Foundation
import NetworkExtension

public enum PrototypeVPNState: Equatable {
    case disconnected, preparing, starting, stopping, connected
    case failed(String)
}

// This controller is confined to the main actor; OS callbacks re-enter that actor.
// It manages only the explicitly marked test profile, never the working client.
@MainActor
public final class PrototypeVPNController {
    struct Operations {
        var load: () async throws -> [NETunnelProviderManager]
        var save: (NETunnelProviderManager) async throws -> Void
        var reload: (NETunnelProviderManager) async throws -> Void
        var start: (NETunnelProviderManager) throws -> Void
        var stop: (NETunnelProviderManager) -> Void
        var remove: (NETunnelProviderManager) async throws -> Void

        static var system: Self {
            Self(
                load: {
                    try await withCheckedThrowingContinuation { continuation in
                        NETunnelProviderManager.loadAllFromPreferences { managers, error in
                            if let error { continuation.resume(throwing: error) }
                            else { continuation.resume(returning: managers ?? []) }
                        }
                    }
                },
                save: { manager in
                    try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                        manager.saveToPreferences { error in
                            if let error { continuation.resume(throwing: error) }
                            else { continuation.resume() }
                        }
                    }
                },
                reload: { manager in
                    try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                        manager.loadFromPreferences { error in
                            if let error { continuation.resume(throwing: error) }
                            else { continuation.resume() }
                        }
                    }
                },
                start: { try $0.connection.startVPNTunnel() },
                stop: { $0.connection.stopVPNTunnel() },
                remove: { manager in
                    try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                        manager.removeFromPreferences { error in
                            if let error { continuation.resume(throwing: error) }
                            else { continuation.resume() }
                        }
                    }
                }
            )
        }
    }

    public private(set) var state: PrototypeVPNState = .disconnected {
        didSet { onChange?(state) }
    }
    public var onChange: ((PrototypeVPNState) -> Void)?
    private let operations: Operations
    private let nativeEngine: Bool
    private let lease: VPNControlLease?
    private var manager: NETunnelProviderManager?
    private var observer: NSObjectProtocol?
    private var operation: UUID?
    private var generation = UUID()
    private var wantsConnection = false

    public convenience init(nativeEngine: Bool = false, lease: VPNControlLease? = nil) { self.init(operations: .system, nativeEngine: nativeEngine, lease: lease) }
    init(operations: Operations, nativeEngine: Bool = false, lease: VPNControlLease? = nil) { self.operations = operations; self.nativeEngine = nativeEngine; self.lease = lease }

    deinit {
        if let observer { NotificationCenter.default.removeObserver(observer) }
    }

    static func select(_ managers: [NETunnelProviderManager]) throws -> NETunnelProviderManager? {
        let owned = managers.filter {
            guard let configuration = $0.protocolConfiguration as? NETunnelProviderProtocol else { return false }
            return configuration.providerBundleIdentifier == ActivationController.identifier
                && configuration.providerConfiguration?["aheadscale.prototype"] as? String == "session-v1"
        }
        guard owned.count <= 1 else {
            throw NSError(domain: "ru.aheadscale.prototype", code: 1,
                          userInfo: [NSLocalizedDescriptionKey: "Найдено несколько тестовых профилей Aheadscale. Автоматический выбор запрещён."])
        }
        return owned.first
    }

    static func makeProfile(nativeEngine: Bool = false) -> NETunnelProviderManager {
        let manager = NETunnelProviderManager()
        let configuration = NETunnelProviderProtocol()
        configuration.providerBundleIdentifier = ActivationController.identifier
        configuration.providerConfiguration = nativeEngine
            ? ["aheadscale.prototype": "session-v1", "aheadscale.native-engine": true]
            : ["aheadscale.prototype": "session-v1", "aheadscale.bridge-proof": true]
        configuration.serverAddress = "aheadscale.ru"
        configuration.includeAllNetworks = false
        manager.protocolConfiguration = configuration
        manager.localizedDescription = "Aheadscale — тест системного VPN"
        manager.isEnabled = true
        manager.isOnDemandEnabled = false
        return manager
    }

    public func connect() {
        guard operation == nil, state != .starting, state != .stopping, state != .connected else { return }
        let token: UUID
        do { token = try beginRequest() } catch { state = .failed("Не удалось согласовать запрос VPN."); return }
        operation = token
        wantsConnection = true
        state = .preparing
        Task { @MainActor in
            defer { if self.operation == token { self.operation = nil } }
            do {
                let profiles = try await operations.load()
                guard current(token) else { return }
                let selected = try Self.select(profiles) ?? Self.makeProfile(nativeEngine: nativeEngine)
                if let configuration = selected.protocolConfiguration as? NETunnelProviderProtocol {
                    configuration.providerConfiguration = nativeEngine
                        ? ["aheadscale.prototype": "session-v1", "aheadscale.native-engine": true]
                        : ["aheadscale.prototype": "session-v1", "aheadscale.bridge-proof": true]
                }
                manager = selected
                if nativeEngine && [.connected, .connecting, .reasserting].contains(selected.connection.status) {
                    observe(selected); state = .starting; consume(selected.connection.status); return
                }
                selected.isEnabled = true
                selected.isOnDemandEnabled = false
                try await operations.save(selected)
                guard current(token) else { return }
                try await operations.reload(selected)
                guard current(token) else { return }
                observe(selected)
                state = .starting
                _ = try mutate(token) { try operations.start(selected) }
            } catch {
                guard current(token) else { return }
                wantsConnection = false
                state = .failed(error.localizedDescription)
            }
        }
    }

    public var requestIsCurrent: Bool { current(generation) }

    private func beginRequest() throws -> UUID {
        let token = UUID()
        try lease?.claim(token)
        generation = token
        return token
    }
    private func current(_ token: UUID) -> Bool {
        guard generation == token else { return false }
        guard let lease else { return true }
        return (try? lease.isCurrent(token)) == true
    }
    private func mutate(_ token: UUID, _ body: () throws -> Void) throws -> Bool {
        guard generation == token else { return false }
        if let lease { return try lease.withCurrent(token, body) }
        try body(); return true
    }

    public func snapshot() async throws -> VPNControlSnapshot {
        let profiles = try await operations.load()
        let selected = try Self.select(profiles)
        let state: String
        switch selected?.connection.status {
        case .connected?: state = "connected"
        case .connecting?, .reasserting?: state = "connecting"
        case .disconnecting?: state = "disconnecting"
        default: state = "disconnected"
        }
        return VPNControlSnapshot(systemState: state, profileExists: selected != nil)
    }

    public func disconnect() {
        wantsConnection = false
        let token: UUID
        do { token = try beginRequest() } catch { state = .failed("Не удалось согласовать отключение VPN."); return }
        if let manager { stop(manager, token: token); return }
        state = .stopping
        Task { @MainActor in
            do {
                let profiles = try await operations.load()
                guard current(token) else { return }
                guard let selected = try Self.select(profiles) else { state = .disconnected; return }
                manager = selected
                observe(selected)
                stop(selected, token: token)
            } catch {
                guard current(token) else { return }
                state = .failed("Не удалось отключить системный VPN.")
            }
        }
    }
    private func stop(_ selected: NETunnelProviderManager, token: UUID) {
        do {
            guard try mutate(token, { operations.stop(selected) }) else { return }
            switch selected.connection.status {
            case .invalid, .disconnected: state = .disconnected
            default: state = .stopping
            }
        } catch { state = .failed("Не удалось отключить системный VPN.") }
    }

    // A timed-out helper may stop only its own request, never claim a newer
    // generation over another process's Disconnect/Connect.
    public func cancelCurrentRequest() {
        let token = generation
        if let manager { _ = try? mutate(token) { operations.stop(manager) } }
        wantsConnection = false
        generation = UUID()
    }

    public func removeProfile() {
        guard operation == nil else { return }
        let token: UUID
        do { token = try beginRequest() } catch { state = .failed("Не удалось согласовать запрос VPN."); return }
        operation = token
        Task { @MainActor in
            defer { if self.operation == token { self.operation = nil } }
            do {
                let profiles = try await operations.load()
                guard current(token) else { return }
                guard let selected = try Self.select(profiles) else { state = .disconnected; return }
                guard selected.connection.status == .disconnected || selected.connection.status == .invalid else {
                    throw NSError(domain: "ru.aheadscale.prototype", code: 2,
                                  userInfo: [NSLocalizedDescriptionKey: "Сначала отключите тестовый VPN."])
                }
                try await operations.remove(selected)
                guard current(token) else { return }
                if let observer { NotificationCenter.default.removeObserver(observer); self.observer = nil }
                manager = nil
                state = .disconnected
            } catch {
                guard current(token) else { return }
                wantsConnection = false
                state = .failed(error.localizedDescription)
            }
        }
    }

    private func observe(_ selected: NETunnelProviderManager) {
        if let observer { NotificationCenter.default.removeObserver(observer) }
        let identity = ObjectIdentifier(selected)
        observer = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange, object: selected.connection, queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                guard let self, let current = self.manager, ObjectIdentifier(current) == identity else { return }
                self.consume(current.connection.status)
            }
        }
    }

    func consume(_ status: NEVPNStatus) {
        guard requestIsCurrent else { return }
        guard wantsConnection else {
            // A queued status notification must never undo an explicit OFF/Cancel.
            if status == .connecting || status == .connected || status == .reasserting {
                if let manager { _ = try? mutate(generation) { operations.stop(manager) } }
            }
            if state == .stopping && (status == .invalid || status == .disconnected) {
                state = .disconnected
            }
            return
        }
        switch status {
        case .connected where nativeEngine:
            state = .connected
        case .reasserting where nativeEngine:
            state = .starting
        case .connected, .reasserting:
            // Until the authenticated engine is attached, Connected is a failure.
            wantsConnection = false
            if let manager { _ = try? mutate(generation) { operations.stop(manager) } }
            state = .failed("Тестовый VPN остановлен: VPN-движок ещё не подключён.")
        case .connecting:
            if state != .stopping { state = .starting }
        case .disconnecting: break // Keep the start failure visible until disconnected.
        case .invalid, .disconnected:
            wantsConnection = false
            state = .failed("Тестовый VPN не запущен: канал к VPN-службе ещё не подключён.")
        @unknown default:
            wantsConnection = false
            if let manager { _ = try? mutate(generation) { operations.stop(manager) } }
            state = .failed("Неизвестный статус системного VPN.")
        }
    }
}
