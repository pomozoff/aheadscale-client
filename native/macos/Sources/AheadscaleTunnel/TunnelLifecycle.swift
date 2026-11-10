import Foundation

// NEProvider cannot be constructed by an unentitled XCTest process. Keep the
// tested lifecycle outside that OS object; native signed checks cover the facade.
final class TunnelLifecycle {
    private final class Pending {
        private let lock = NSLock()
        private var completion: ((Error?) -> Void)?
        // Accessed only under TunnelLifecycle.lock.
        var session: ProviderProofSession?
        init(_ completion: @escaping (Error?) -> Void) { self.completion = completion }
        func finish(_ error: Error) {
            lock.lock()
            let completion = self.completion
            self.completion = nil
            lock.unlock()
            completion?(error)
        }
    }
    private let lock = NSLock()
    private var pending: Pending?

    func start(makeProof: ((@escaping (Result<Void, Error>) -> Void) -> ProviderProofSession)? = nil,
               completion: @escaping (Error?) -> Void) {
        guard let makeProof else {
            completion(Self.failure(1, "Aheadscale: канал к VPN-службе ещё не подключён. Это тестовое расширение."))
            return
        }
        let attempt = Pending(completion)
        lock.lock()
        guard pending == nil else {
            lock.unlock()
            attempt.finish(Self.failure(3, "Aheadscale: проверка уже выполняется."))
            return
        }
        pending = attempt
        lock.unlock()
        let session = makeProof { [weak self] result in
            if let self {
                self.lock.lock()
                if self.pending === attempt { self.pending = nil }
                self.lock.unlock()
            }
            let message: String
            switch result {
            case .success:
                NSLog("Aheadscale provider bridge proof: IPv4/IPv6 PASS; engine not attached")
                message = "Aheadscale: защищённый канал к root-службе проверен; VPN-движок ещё не подключён."
            case .failure:
                NSLog("Aheadscale provider bridge proof: refused or cancelled")
                message = "Aheadscale: проверка защищённого канала не завершена; VPN не запущен."
            }
            attempt.finish(Self.failure(2, message))
        }
        lock.lock()
        guard pending === attempt else {
            lock.unlock()
            session.stop()
            attempt.finish(Self.failure(4, "Aheadscale: подключение отменено."))
            return
        }
        attempt.session = session
        lock.unlock()
        session.start()
    }

    func stop(completion: @escaping () -> Void) {
        lock.lock()
        let attempt = pending
        let session = attempt?.session
        pending = nil
        lock.unlock()
        session?.stop()
        attempt?.finish(Self.failure(4, "Aheadscale: подключение отменено."))
        completion()
    }

    private static func failure(_ code: Int, _ message: String) -> NSError {
        NSError(domain: "ru.aheadscale.client.tunnel", code: code,
                userInfo: [NSLocalizedDescriptionKey: message])
    }
}
