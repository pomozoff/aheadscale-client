import Foundation

public enum VPNControlAction: String, Codable {
    case status, connect, disconnect, cancel
    public static func parse(_ arguments: [String]) throws -> Self {
        guard arguments.count == 2, arguments[0] == "--vpn-control",
              let action = Self(rawValue: arguments[1]) else {
            throw NSError(domain: "ru.aheadscale.control", code: 1)
        }
        return action
    }
}

// This is the OS state only. GUI/CLI must also check the engine before showing
// product-level Connected; a status query never modifies preferences or intent.
public struct VPNControlSnapshot: Codable {
    public let version: Int
    public let systemState: String
    public let profileExists: Bool
    public init(systemState: String, profileExists: Bool) {
        self.version = 1; self.systemState = systemState; self.profileExists = profileExists
    }
}
