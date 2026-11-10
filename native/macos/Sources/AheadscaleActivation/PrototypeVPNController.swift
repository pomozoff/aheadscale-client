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
    private var manager: NETunnelProviderManager?
    private var observer: NSObjectProtocol?
    private var operation: UUID?
    private var generation = UUID()
    private var wantsConnection = false

    public convenience init(nativeEngine: Bool = false) { self.init(operations: .system, nativeEngine: nativeEngine) }
    init(operations: Operations, nativeEngine: Bool = false) { self.operations = operations; self.nativeEngine = nativeEngine }

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
        let token = UUID()
        generation = token
        operation = token
        wantsConnection = true
        state = .preparing
        Task { @MainActor in
            defer { if self.operation == token { self.operation = nil } }
            do {
                let profiles = try await operations.load()
                guard generation == token else { return }
                let selected = try Self.select(profiles) ?? Self.makeProfile(nativeEngine: nativeEngine)
                if let configuration = selected.protocolConfiguration as? NETunnelProviderProtocol {
                    configuration.providerConfiguration = nativeEngine
                        ? ["aheadscale.prototype": "session-v1", "aheadscale.native-engine": true]
                        : ["aheadscale.prototype": "session-v1", "aheadscale.bridge-proof": true]
                }
                manager = selected
                selected.isEnabled = true
                selected.isOnDemandEnabled = false
                try await operations.save(selected)
                guard generation == token else { return }
                try await operations.reload(selected)
                guard generation == token else { return }
                observe(selected)
                state = .starting
                try operations.start(selected)
            } catch {
                guard generation == token else { return }
                wantsConnection = false
                state = .failed(error.localizedDescription)
            }
        }
    }

    public func disconnect() {
        wantsConnection = false
        generation = UUID()
        guard let manager else { state = .disconnected; return }
        operations.stop(manager)
        switch manager.connection.status {
        case .invalid, .disconnected: state = .disconnected
        default: state = .stopping
        }
    }

    public func removeProfile() {
        guard operation == nil else { return }
        let token = UUID()
        generation = token
        operation = token
        Task { @MainActor in
            defer { if self.operation == token { self.operation = nil } }
            do {
                let profiles = try await operations.load()
                guard generation == token else { return }
                guard let selected = try Self.select(profiles) else { state = .disconnected; return }
                guard selected.connection.status == .disconnected || selected.connection.status == .invalid else {
                    throw NSError(domain: "ru.aheadscale.prototype", code: 2,
                                  userInfo: [NSLocalizedDescriptionKey: "Сначала отключите тестовый VPN."])
                }
                try await operations.remove(selected)
                guard generation == token else { return }
                if let observer { NotificationCenter.default.removeObserver(observer); self.observer = nil }
                manager = nil
                state = .disconnected
            } catch {
                guard generation == token else { return }
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
        guard wantsConnection else {
            // A queued status notification must never undo an explicit OFF/Cancel.
            if status == .connecting || status == .connected || status == .reasserting {
                if let manager { operations.stop(manager) }
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
            if let manager { operations.stop(manager) }
            state = .failed("Тестовый VPN остановлен: VPN-движок ещё не подключён.")
        case .connecting:
            if state != .stopping { state = .starting }
        case .disconnecting: break // Keep the start failure visible until disconnected.
        case .invalid, .disconnected:
            wantsConnection = false
            state = .failed("Тестовый VPN не запущен: канал к VPN-службе ещё не подключён.")
        @unknown default:
            wantsConnection = false
            if let manager { operations.stop(manager) }
            state = .failed("Неизвестный статус системного VPN.")
        }
    }
}
