import Foundation

// A Team-prefixed macOS group needs no new registered Apple App Group.
// The echo helper has a test-only identity, distinct from the future Go broker.
public enum RootBridgeProofIdentity {
    public static let group = "Y24464MUL3.ru.aheadscale.client"
    public static let service = group + ".bridgeprobe"
    public static let brokerRequirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.probe.rootbroker\""
    public static let providerRequirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.client.tunnel\""
}
