import Foundation
import Darwin
import AheadscaleActivation

@main
struct VPNControlMain {
    @MainActor static func main() async {
        do {
            guard geteuid() != 0 else { throw NSError(domain: "ru.aheadscale.control", code: 3) }
            let action = try VPNControlAction.parse(Array(CommandLine.arguments.dropFirst()))
            let lease = try VPNControlLease.userLease()
            let controller = PrototypeVPNController(nativeEngine: true, lease: lease)
            if action == .status { try output(try await controller.snapshot()); return }
            if action == .connect { controller.connect() } else { controller.disconnect() }
            let deadline = ProcessInfo.processInfo.systemUptime + 125
            while ProcessInfo.processInfo.systemUptime < deadline {
                guard controller.requestIsCurrent else { failure("superseded", code: 3) }
                switch controller.state {
                case .connected where action == .connect:
                    try output(try await controller.snapshot()); return
                case .disconnected where action != .connect:
                    try output(try await controller.snapshot()); return
                case .failed:
                    failure("control_failed", code: 1)
                default: break
                }
                try await Task.sleep(nanoseconds: 100_000_000)
            }
            controller.cancelCurrentRequest()
            failure("timeout", code: 1)
        } catch { failure("control_unavailable", code: 1) }
    }
    private static func output(_ snapshot: VPNControlSnapshot) throws {
        let data = try JSONEncoder().encode(snapshot)
        guard data.count <= 4096 else { throw NSError(domain: "ru.aheadscale.control", code: 4) }
        FileHandle.standardOutput.write(data + Data([10]))
    }
    private static func failure(_ error: String, code: Int32) -> Never {
        // Fixed vocabulary only: no underlying OS errors, tokens or config paths.
        let value: [String: Any] = ["version": 1, "systemState": "unknown", "profileExists": false, "error": error]
        let data = try! JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
        FileHandle.standardOutput.write(data + Data([10]))
        exit(code)
    }
}
