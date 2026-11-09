// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "AheadscaleNetworkExtension",
    platforms: [.macOS(.v11)],
    products: [
        .executable(name: "AheadscaleTunnelExtension", targets: ["TunnelExtension"]),
        .executable(name: "AheadscaleExtensionSetup", targets: ["ActivationApp"])
    ],
    targets: [
        .target(name: "AheadscaleTunnel"),
        .target(name: "AheadscaleActivation"),
        .executableTarget(name: "ActivationApp", dependencies: ["AheadscaleActivation"]),
        .executableTarget(name: "TunnelExtension", dependencies: ["AheadscaleTunnel"]),
        .testTarget(name: "AheadscaleTunnelTests", dependencies: ["AheadscaleTunnel"]),
        .testTarget(name: "AheadscaleActivationTests", dependencies: ["AheadscaleActivation"])
    ],
    swiftLanguageModes: [.v5]
)
