import Foundation
import XCTest

struct RepoRule: Sendable {
    let name: String
    let roots: [String]
    let pattern: String
    let message: String
    let failing: [String]
    let passing: [String]
    var applies: @Sendable (_ path: String, _ contents: String) -> Bool = { _, _ in true }

    func matches(in text: String) throws -> Int {
        let regex = try NSRegularExpression(pattern: pattern)
        return regex.numberOfMatches(in: text, range: NSRange(text.startIndex..., in: text))
    }
}

enum RepoRules {
    static let base58 = "(?<![1-9A-HJ-NP-Za-km-z])[1-9A-HJ-NP-Za-km-z]{32,44}(?![1-9A-HJ-NP-Za-km-z])"
    static let fakeMint = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"
    static let productCode = ["apps/mobile/Monaco", "packages/mobile-core/Sources"]
    static let testCode = ["apps/mobile/MonacoTests", "packages/mobile-core/Tests"]

    static let all: [RepoRule] = [
        RepoRule(
            name: "copy",
            roots: productCode,
            pattern:
                #"(?:\b(?:Text|Button|Label|ToastCopy|MonacoToast)\(|\.navigationTitle\(|\bString\(localized:)\s*(?:\w+:\s*)?"#
                + #""[^"\n]*(?:(?i:xstock)|(?i:\bclub\b)|(?i:\bgroup\b)|"# + base58 + #")[^"\n]*""#,
            message: "User-facing copy says cabal, never club or group, and shows no xStock branding or raw address.",
            failing: [
                #"Text("Join the club")"#,
                #"Text("xStock")"#,
                #"Button("Leave group") {"#,
                #".navigationTitle("Buy XSTOCK")"#,
                #"Label(title: "Mint \#(fakeMint)")"#,
                #"Text("\#(fakeMint)")"#,
                #"MonacoToast(message: "Group funded")"#,
            ],
            passing: [
                #"Text("Join the cabal")"#,
                #"Text("Groups of stocks")"#,
                #"Text(verbatim: address)"#,
                #"let group = "Join the club""#,
            ]
        ),
        RepoRule(
            name: "print",
            roots: productCode,
            pattern: #"\bprint\("#,
            message: "No print( in app code, #if DEBUG included. Use Logger.",
            failing: [#"print("hi")"#, #"Swift.print(value)"#],
            passing: [#"logger.info("hi")"#, "let digest = fingerprint(of: request)"]
        ),
        RepoRule(
            name: "test-sleep",
            roots: testCode,
            pattern: #"\bTask\.sleep\b|\basyncAfter\b|\bwait\(for:"#,
            message: "Tests wait on a signal or an injected clock, never on wall time.",
            failing: [
                "try await Task.sleep(for: .milliseconds(10))",
                "DispatchQueue.main.asyncAfter(deadline: .now() + 1) {",
                "wait(for: [expectation], timeout: 1)",
            ],
            passing: ["await clock.advance(by: .seconds(1))", "await fulfillment(of: [expectation])"]
        ),
        RepoRule(
            name: "toast-copy",
            roots: productCode,
            pattern: #""[^"\n]*(?:(?i:xstock)|(?i:\bclub\b)|(?i:\bgroup\b)|"# + base58 + #")[^"\n]*""#,
            message: "Toast copy says cabal, never club or group, and shows no xStock branding or raw address.",
            failing: [
                #""Group funded""#,
                #""\#(fakeMint)""#,
            ],
            passing: [
                #""Cabal funded""#
            ],
            applies: { path, _ in (path as NSString).lastPathComponent == "ToastCopy.swift" }
        ),
        RepoRule(
            name: "fixture-address",
            roots: ["packages/mobile-core/Sources/MonacoAPI/Fixtures"],
            pattern: #""[^"\n]*"# + base58 + #"[^"\n]*""#,
            message: "Fixtures use placeholders, never a base58 address.",
            failing: [
                #""\#(fakeMint)""#,
                #"let mint = "\#(fakeMint)""#,
            ],
            passing: [
                #""USDC""#,
                "let mint = placeholder",
            ]
        ),
        RepoRule(
            name: "wall-clock",
            roots: ["packages/mobile-core/Sources/MonacoCore"],
            pattern: #"\bDate\(\)|\bDate\.now\b|\bContinuousClock\(\)|\bContinuousClock\.now\b"#,
            message: "MonacoCore takes an injected clock. Date() and ContinuousClock() stay in *Clock.swift.",
            failing: [
                "let now = Date()",
                "let now = Date.now",
                "let clock = ContinuousClock()",
                "let now = ContinuousClock.now",
            ],
            passing: [
                "let now = clock.now",
                "let now = myDate.now",
            ],
            applies: { path, _ in !(path as NSString).lastPathComponent.hasSuffix("Clock.swift") }
        ),
        RepoRule(
            name: "float-money",
            roots: productCode,
            pattern: #"\bDouble\(|\bDecimal\("#,
            message: "Money is integer micros until a formatter renders it.",
            failing: [
                "let amount = Double(micros)",
                "let amount = Decimal(1)",
            ],
            passing: [
                "let amount = Int(micros)",
                "let cents = micros / 100",
            ],
            applies: { path, contents in
                let inDomain = path.range(of: #"Sources/MonacoCore/[^/]+/"#, options: .regularExpression) != nil
                let networking = path.contains("Sources/MonacoCore/Networking/")
                let micros = contents.range(of: "micros", options: .caseInsensitive) != nil
                return (inDomain && !networking) || micros
            }
        ),
        RepoRule(
            name: "urlsession",
            roots: productCode,
            pattern: #"\bURLSession\b"#,
            message: "URLSession stays in MonacoAPI. The app uses the Monaco client.",
            failing: ["let session = URLSession.shared"],
            passing: ["let request = URLRequest(url: url)", "URLSessionConfiguration.default"],
            applies: { path, _ in !path.hasPrefix("packages/mobile-core/Sources/MonacoAPI/") }
        ),
        RepoRule(
            name: "unchecked-sendable",
            roots: ["apps/mobile", "packages/mobile-core"],
            pattern: #"@unchecked\s+Sendable"#,
            message: "New @unchecked Sendable needs a reason that already has a row, and the row only shrinks.",
            failing: ["final class Box: @unchecked Sendable {"],
            passing: ["final class Box: Sendable {"]
        ),
        RepoRule(
            name: "nonisolated-unsafe",
            roots: ["apps/mobile", "packages/mobile-core"],
            pattern: #"nonisolated\s*\(\s*unsafe\s*\)"#,
            message: "New nonisolated(unsafe) needs a reason that already has a row, and the row only shrinks.",
            failing: ["nonisolated(unsafe) var cache: [URL: Data] = [:]"],
            passing: ["nonisolated var cache: [URL: Data] = [:]"]
        ),
        RepoRule(
            name: "lint-directive",
            roots: ["apps/mobile", "packages/mobile-core"],
            pattern: #"swiftlint:|swift-format-ignore"#,
            message: "A lint directive would hide no_comments. Name the thing instead.",
            failing: [
                "// swiftlint:disable no_comments",
                "// swift-format-ignore",
            ],
            passing: [
                "let ruleName = swiftlint",
                "func ignore() {}",
            ]
        ),
        RepoRule(
            name: "observable-object",
            roots: productCode,
            pattern: #"\bObservableObject\b|@Published\b"#,
            message: "Screen state is an @Observable model. ObservableObject and @Published only shrink.",
            failing: [
                "final class Foo: ObservableObject {",
                "@Published var name = \"\"",
            ],
            passing: [
                "@Observable final class Foo {",
                "var name = \"\"",
            ]
        ),
        RepoRule(
            name: "external-host",
            roots: productCode,
            pattern: #""[^"\n]*(?:jup\.ag|api\.xstocks\.fi|pyth\.network|solana\.com)[^"\n]*""#,
            message: "The app talks to the Monaco API only. Explorer links stay on solscan.io.",
            failing: [
                #"let url = "https://lite-api.jup.ag/swap/v1/quote""#,
                #"let url = "https://api.xstocks.fi/v1/assets""#,
                #"let url = "https://hermes.pyth.network/v2/updates""#,
                #"let url = "https://api.mainnet-beta.solana.com""#,
            ],
            passing: [
                #"let url = "https://solscan.io/tx/abc""#,
                #"let url = "https://api.monaco.app/v1/cabals""#,
            ]
        ),
    ]
}

enum RepoTree {
    static let root = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
    static let allowlist = "packages/mobile-core/Tests/MonacoCoreTests/RepoRulesAllowlist.txt"
    static let ownFile = "packages/mobile-core/Tests/MonacoCoreTests/RepoRulesTests.swift"

    static func swiftFiles(under directory: String) -> [String] {
        let base = root.appendingPathComponent(directory).path
        guard let walk = FileManager.default.enumerator(atPath: base) else { return [] }
        var files: [String] = []
        while let relative = walk.nextObject() as? String {
            if (relative as NSString).lastPathComponent == ".build" {
                walk.skipDescendants()
            } else if relative.hasSuffix(".swift") {
                files.append(directory + "/" + relative)
            }
        }
        return files.filter { $0 != ownFile }
    }

    static func read(_ path: String) throws -> String {
        try String(contentsOf: root.appendingPathComponent(path), encoding: .utf8)
    }

    static func counts(for rule: RepoRule) throws -> [String: Int] {
        var counts: [String: Int] = [:]
        for path in rule.roots.flatMap(swiftFiles(under:)) {
            let contents = try read(path)
            guard rule.applies(path, contents) else { continue }
            let found = try rule.matches(in: contents)
            if found > 0 {
                counts[path] = found
            }
        }
        return counts
    }

    static func allowed() throws -> [String: [String: Int]] {
        var rows: [String: [String: Int]] = [:]
        for line in try read(allowlist).split(separator: "\n") {
            let fields = line.split(separator: "\t").map(String.init)
            guard fields.count == 3, let count = Int(fields[2]) else {
                throw RepoRulesError.malformedRow(String(line))
            }
            rows[fields[0], default: [:]][fields[1]] = count
        }
        return rows
    }
}

enum RepoRulesError: Error {
    case malformedRow(String)
}

final class RepoRulesTests: XCTestCase {
    func testEveryRuleFlagsItsFailingFixturesAndPassesItsPassingOnes() throws {
        for rule in RepoRules.all {
            for fixture in rule.failing {
                XCTAssertGreaterThan(try rule.matches(in: fixture), 0, "\(rule.name) should flag: \(fixture)")
            }
            for fixture in rule.passing {
                XCTAssertEqual(try rule.matches(in: fixture), 0, "\(rule.name) should pass: \(fixture)")
            }
        }
    }

    func testTheTreeStaysWithinTheAllowlist() throws {
        let allowed = try RepoTree.allowed()
        let names = Set(RepoRules.all.map(\.name))
        for rule in allowed.keys where !names.contains(rule) {
            XCTFail("\(RepoTree.allowlist) has rows for unknown rule \(rule)")
        }
        for rule in RepoRules.all {
            let found = try RepoTree.counts(for: rule)
            let rows = allowed[rule.name, default: [:]]
            for (path, count) in found.sorted(by: { $0.key < $1.key }) where count > rows[path, default: 0] {
                XCTFail("\(rule.name) \(path) \(rows[path, default: 0]) -> \(count): \(rule.message)")
            }
            for (path, count) in rows.sorted(by: { $0.key < $1.key }) where found[path, default: 0] < count {
                XCTFail("lower the allowlist: \(rule.name) \(path) \(count) -> \(found[path, default: 0])")
            }
        }
    }

    func testTheRepoRootHoldsBothSwiftTrees() {
        XCTAssertFalse(RepoTree.swiftFiles(under: "apps/mobile/Monaco").isEmpty)
        XCTAssertFalse(RepoTree.swiftFiles(under: "packages/mobile-core/Sources").isEmpty)
    }
}
