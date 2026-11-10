// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "AheadscaleNetworkExtension",
    platforms: [.macOS(.v11)],
    products: [
        .executable(name: "AheadscaleTunnelExtension", targets: ["TunnelExtension"]),
        .executable(name: "AheadscaleExtensionSetup", targets: ["ActivationApp"]),
        .executable(name: "AheadscaleRootBridgePrototype", targets: ["RootBridgePrototype"]),
        .executable(name: "AheadscaleBridgeProbeRunner", targets: ["BridgeProbeRunner"])
    ],
    targets: [
        .target(name: "AheadscaleTunnel", dependencies: ["AheadscaleBridgeProbe"]),
        .target(name: "AheadscaleActivation"),
        .target(name: "AheadscaleBridgeProbe"),
        .executableTarget(name: "BridgeProbeRunner", dependencies: ["AheadscaleBridgeProbe"]),
        .executableTarget(name: "ActivationApp", dependencies: ["AheadscaleActivation"]),
        .executableTarget(name: "RootBridgePrototype", dependencies: ["AheadscaleBridgeProbe"]),
        .executableTarget(name: "TunnelExtension", dependencies: ["AheadscaleTunnel"]),
        .testTarget(name: "AheadscaleTunnelTests", dependencies: ["AheadscaleTunnel", "AheadscaleBridgeProbe"]),
        .testTarget(name: "AheadscaleActivationTests", dependencies: ["AheadscaleActivation"]),
        .testTarget(name: "AheadscaleBridgeProbeTests", dependencies: ["AheadscaleBridgeProbe"])
    ],
    swiftLanguageModes: [.v5]
)
