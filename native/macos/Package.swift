// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "AheadscaleNetworkExtension",
    platforms: [.macOS(.v11)],
    products: [
        .executable(name: "AheadscaleTunnelExtension", targets: ["TunnelExtension"]),
        .executable(name: "AheadscaleExtensionSetup", targets: ["ActivationApp"]),
        .executable(name: "AheadscaleBridgeProbeRunner", targets: ["BridgeProbeRunner"])
    ],
    targets: [
        .target(name: "AheadscaleTunnel"),
        .target(name: "AheadscaleActivation"),
        .target(name: "AheadscaleBridgeProbe"),
        .executableTarget(name: "BridgeProbeRunner", dependencies: ["AheadscaleBridgeProbe"]),
        .executableTarget(name: "ActivationApp", dependencies: ["AheadscaleActivation"]),
        .executableTarget(name: "TunnelExtension", dependencies: ["AheadscaleTunnel"]),
        .testTarget(name: "AheadscaleTunnelTests", dependencies: ["AheadscaleTunnel"]),
        .testTarget(name: "AheadscaleActivationTests", dependencies: ["AheadscaleActivation"]),
        .testTarget(name: "AheadscaleBridgeProbeTests", dependencies: ["AheadscaleBridgeProbe"])
    ],
    swiftLanguageModes: [.v5]
)
