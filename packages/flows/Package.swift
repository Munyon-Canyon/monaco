// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "MonacoFlows",
    platforms: [
        .macOS(.v15),
        .iOS(.v18),
    ],
    products: [
        .library(name: "MonacoFlows", targets: ["MonacoFlows"])
    ],
    targets: [
        .target(name: "MonacoFlows")
    ],
    swiftLanguageModes: [.v6]
)
