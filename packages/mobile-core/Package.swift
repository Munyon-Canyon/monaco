// swift-tools-version: 6.2
import PackageDescription

let moduleGraph: [(name: String, imports: [String])] = [
    ("MonacoSystem", []),
    ("MonacoIdentity", []),
    ("MonacoAnalytics", []),
    ("MonacoMarket", []),
    ("MonacoNotify", ["MonacoIdentity"]),
    ("MonacoReferrals", ["MonacoIdentity"]),
    ("MonacoCabal", ["MonacoIdentity"]),
    ("MonacoSocial", ["MonacoIdentity", "MonacoCabal"]),
    ("MonacoTreasury", ["MonacoCabal"]),
    ("MonacoTrading", ["MonacoTreasury", "MonacoMarket"]),
    ("MonacoGovernance", ["MonacoCabal", "MonacoTreasury"]),
    ("MonacoRanking", ["MonacoCabal", "MonacoMarket", "MonacoTrading"]),
]
let moduleNames = moduleGraph.map { Target.Dependency(stringLiteral: $0.name) }
let moduleTargets: [Target] = moduleGraph.map { module in
    .target(
        name: module.name,
        dependencies: ["MonacoAPI", .product(name: "MonacoFlows", package: "flows")]
            + module.imports.map { Target.Dependency(stringLiteral: $0) }
    )
}

let package = Package(
    name: "MonacoCore",
    platforms: [
        .macOS(.v15),
        .iOS(.v18),
    ],
    products: [
        .library(name: "MonacoCore", targets: ["MonacoCore"]),
        .library(name: "MonacoAPI", targets: ["MonacoAPI"]),
        .library(name: "MonacoTestClock", targets: ["MonacoTestClock"]),
        .library(name: "MonacoTestSupport", targets: ["MonacoTestSupport"]),
    ],
    dependencies: [
        .package(path: "../flows"),
        .package(url: "https://github.com/apple/swift-http-types", from: "1.8.0"),
        .package(url: "https://github.com/apple/swift-openapi-generator", exact: "1.13.0"),
        .package(url: "https://github.com/apple/swift-openapi-runtime", from: "1.12.1"),
        .package(url: "https://github.com/apple/swift-openapi-urlsession", from: "1.3.1"),
        .package(url: "https://github.com/swift-server/swift-openapi-async-http-client", exact: "1.5.0"),
    ],
    targets: [
        .target(
            name: "MonacoCore",
            dependencies: ["MonacoAPI", .product(name: "MonacoFlows", package: "flows")] + moduleNames
        ),
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
                .plugin(name: "OpenAPIGenerator", package: "swift-openapi-generator")
            ]
        ),
        .target(name: "MonacoTestClock"),
        .target(
            name: "MonacoTestSupport",
            dependencies: [
                "MonacoAPI",
                .product(name: "HTTPTypes", package: "swift-http-types"),
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
            ]
        ),
        .testTarget(
            name: "MonacoCoreTests",
            dependencies: [
                "MonacoCore", "MonacoAPI", "MonacoTestClock", "MonacoTestSupport",
                .product(name: "MonacoFlows", package: "flows"),
                .product(
                    name: "OpenAPIAsyncHTTPClient", package: "swift-openapi-async-http-client",
                    condition: .when(platforms: [.linux])
                ),
            ] + moduleNames,
            exclude: ["RepoRulesAllowlist.txt"],
            resources: [
                .process("Fixtures")
            ]
        ),
        .testTarget(
            name: "MonacoAPITests",
            dependencies: [
                "MonacoAPI",
                "MonacoTestClock",
                "MonacoTestSupport",
                .product(name: "HTTPTypes", package: "swift-http-types"),
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
                .product(
                    name: "OpenAPIAsyncHTTPClient", package: "swift-openapi-async-http-client",
                    condition: .when(platforms: [.linux])
                ),
            ]
        ),
    ] + moduleTargets,
    swiftLanguageModes: [.v6]
)
