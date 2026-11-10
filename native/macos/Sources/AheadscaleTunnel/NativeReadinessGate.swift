import Foundation

// All three acknowledgements belong to the root-issued session generation.
// The gate does not perform IO. Callbacks must be short and nonblocking; they
// run serially under a recursive delivery lock so Cancel cannot overtake Ready
// and a callback can cancel re-entrantly without deadlocking.
final class NativeReadinessGate: @unchecked Sendable {
    enum Part: Hashable { case channel, settings, engine }
    private enum State { case preparing, ready, stopped }
    private let generation: UUID
    private let lock = NSRecursiveLock()
    private var parts = Set<Part>()
    private var state: State = .preparing
    private let onReady: () -> Void
    private let onStop: (Error) -> Void

    init(generation: UUID, onReady: @escaping () -> Void, onStop: @escaping (Error) -> Void) {
        self.generation = generation
        self.onReady = onReady
        self.onStop = onStop
    }

    func mark(_ part: Part, generation: UUID) {
        lock.lock()
        defer { lock.unlock() }
        guard generation == self.generation, state == .preparing else { return }
        parts.insert(part)
        guard parts.count == 3 else { return }
        state = .ready
        onReady()
    }

    func fail(_ error: Error, generation: UUID) {
        lock.lock()
        defer { lock.unlock() }
        guard generation == self.generation else { return }
        finish(error)
    }

    func cancel() {
        lock.lock()
        defer { lock.unlock() }
        finish(NSError(domain: "ru.aheadscale.client.tunnel", code: 4,
                       userInfo: [NSLocalizedDescriptionKey: "Aheadscale: подключение отменено."]))
    }

    private func finish(_ error: Error) {
        guard state != .stopped else { return }
        state = .stopped
        parts.removeAll()
        onStop(error)
    }
}
