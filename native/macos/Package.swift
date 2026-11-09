// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "AheadscaleNetworkExtension",
    platforms: [.macOS(.v11)],
    products: [.executable(name: "AheadscaleTunnelExtension", targets: ["TunnelExtension"])],
    targets: [
        .target(name: "AheadscaleTunnel"),
        .executableTarget(name: "TunnelExtension", dependencies: ["AheadscaleTunnel"]),
        .testTarget(name: "AheadscaleTunnelTests", dependencies: ["AheadscaleTunnel"])
    ],
    swiftLanguageModes: [.v5]
)
