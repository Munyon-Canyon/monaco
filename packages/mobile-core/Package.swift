// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "MonacoCore",
    platforms: [
        .macOS(.v13),
        .iOS(.v17),
    ],
    products: [
        .library(name: "MonacoCore", targets: ["MonacoCore"]),
        .library(name: "MonacoAPI", targets: ["MonacoAPI"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-http-types", from: "1.8.0"),
        .package(url: "https://github.com/apple/swift-openapi-generator", from: "1.13.1"),
        .package(url: "https://github.com/apple/swift-openapi-runtime", from: "1.12.1"),
        .package(url: "https://github.com/apple/swift-openapi-urlsession", from: "1.3.1"),
    ],
    targets: [
        .target(name: "MonacoCore"),
        // Sources/MonacoAPI/openapi.yaml is a symlink to apps/backend/api/openapi.yaml, not a
        // copy: the generator plugin reads it through the sandbox in swift build, Xcode and the
        // Linux container, so there is no second file to keep fresh.
        .target(
            name: "MonacoAPI",
            dependencies: [
                .product(name: "HTTPTypes", package: "swift-http-types"),
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
            ],
            plugins: [
                .plugin(name: "OpenAPIGenerator", package: "swift-openapi-generator"),
            ]
        ),
        .testTarget(
            name: "MonacoCoreTests",
            dependencies: ["MonacoCore"],
            resources: [
                .process("Fixtures"),
            ]
        ),
        .testTarget(
            name: "MonacoAPITests",
            dependencies: [
                "MonacoAPI",
                .product(name: "HTTPTypes", package: "swift-http-types"),
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
            ]
        ),
    ]
)
