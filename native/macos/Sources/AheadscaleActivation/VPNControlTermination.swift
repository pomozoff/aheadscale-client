import Dispatch
import Darwin

public enum VPNControlTermination {
    public static func install(_ handler: @escaping @Sendable () -> Void) -> DispatchSourceSignal {
        signal(SIGTERM, SIG_IGN)
        let source = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .global())
        source.setEventHandler { }
        source.resume()
        return source
    }
}
