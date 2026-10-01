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

private struct RequiredReasonAPI {
    let token: String
    let category: String
    let regex: NSRegularExpression
}

private enum RequiredReason {
    static let manifestPath = "apps/mobile/Monaco/PrivacyInfo.xcprivacy"
    static let roots = ["apps/mobile/Monaco", "packages/mobile-core/Sources"]

    private static let tokens: [(token: String, category: String, pattern: String)] = [
        ("UserDefaults", "NSPrivacyAccessedAPICategoryUserDefaults", #"\bUserDefaults\b"#),
        ("@AppStorage", "NSPrivacyAccessedAPICategoryUserDefaults", #"@AppStorage\b"#),
        ("systemUptime", "NSPrivacyAccessedAPICategorySystemBootTime", #"\bsystemUptime\b"#),
        ("mach_absolute_time", "NSPrivacyAccessedAPICategorySystemBootTime", #"\bmach_absolute_time\b"#),
        (
            "creationDate",
            "NSPrivacyAccessedAPICategoryFileTimestamp",
            #"\b"# + "creation" + "Date" + #"(?:Key)?\b"#
        ),
        (
            "modificationDate",
            "NSPrivacyAccessedAPICategoryFileTimestamp",
            #"\b"# + "modification" + "Date" + #"(?:Key)?\b"#
        ),
        (
            "contentModificationDate",
            "NSPrivacyAccessedAPICategoryFileTimestamp",
            #"\b"# + "content" + "Modification" + "Date" + #"(?:Key)?\b"#
        ),
        ("attributesOfItem", "NSPrivacyAccessedAPICategoryFileTimestamp", #"\battributesOfItem\b"#),
        ("getattrlist", "NSPrivacyAccessedAPICategoryFileTimestamp", #"\bgetattrlist\b"#),
        (
            "volumeAvailableCapacity",
            "NSPrivacyAccessedAPICategoryDiskSpace",
            #"\b"# + "volume" + "AvailableCapacity"
                + #"(?:ForImportantUsage|ForOpportunisticUsage)?(?:Key)?\b"#
        ),
        ("systemFreeSize", "NSPrivacyAccessedAPICategoryDiskSpace", #"\bsystemFreeSize\b"#),
        ("statfs", "NSPrivacyAccessedAPICategoryDiskSpace", #"\bstatfs\b"#),
        ("activeInputModes", "NSPrivacyAccessedAPICategoryActiveKeyboards", #"\bactiveInputModes\b"#),
    ]

    static func categories(in plist: Data) throws -> Set<String> {
        let root = try PropertyListSerialization.propertyList(from: plist, options: [], format: nil)
        guard let dict = root as? [String: Any] else { return [] }
        let types = dict["NSPrivacyAccessedAPITypes"] as? [[String: Any]] ?? []
        return Set(types.compactMap { $0["NSPrivacyAccessedAPIType"] as? String })
    }

    static func violations(files: [(path: String, text: String)], declared: Set<String>) throws -> [String] {
        let apis = try tokens.map { token, category, pattern in
            RequiredReasonAPI(token: token, category: category, regex: try NSRegularExpression(pattern: pattern))
        }
        var used = Set<String>()
        var missing: [String] = []
        for file in files.sorted(by: { $0.path < $1.path }) {
            record(file, apis: apis, declared: declared, used: &used, missing: &missing)
        }
        return missing + declared.subtracting(used).sorted().map { "unused category \($0)" }
    }

    private static func record(
        _ file: (path: String, text: String),
        apis: [RequiredReasonAPI],
        declared: Set<String>,
        used: inout Set<String>,
        missing: inout [String]
    ) {
        let rows = file.text.split(separator: "\n", omittingEmptySubsequences: false)
        for (offset, row) in rows.enumerated() {
            let line = String(row)
            let range = NSRange(line.startIndex..., in: line)
            for api in apis where api.regex.firstMatch(in: line, range: range) != nil {
                used.insert(api.category)
                guard declared.contains(api.category) else {
                    missing.append(need(file.path, line: offset + 1, api: api))
                    continue
                }
            }
        }
    }

    private static func need(_ path: String, line: Int, api: RequiredReasonAPI) -> String {
        "\(path):\(line): \(api.token) needs \(api.category) in PrivacyInfo.xcprivacy"
    }
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

    func testThePrivacyManifestCoversEveryRequiredReasonTheAppUses() throws {
        let plist = try Data(contentsOf: RepoTree.root.appendingPathComponent(RequiredReason.manifestPath))
        let declared = try RequiredReason.categories(in: plist)
        var files: [(path: String, text: String)] = []
        for root in RequiredReason.roots {
            for path in RepoTree.swiftFiles(under: root) {
                files.append((path, try RepoTree.read(path)))
            }
        }
        XCTAssertEqual(try RequiredReason.violations(files: files, declared: declared), [])
    }

    func testRequiredReasonNamesTheCategoryACallNeeds() throws {
        let boot = "NSPrivacyAccessedAPICategorySystemBootTime"
        let disk = "NSPrivacyAccessedAPICategoryDiskSpace"
        let plant = "apps/mobile/Monaco/Plant.swift"
        let uptime = [
            (
                path: plant,
                text: "let defaults = UserDefaults.standard\nlet t = ProcessInfo.processInfo.systemUptime\n"
            )
        ]
        XCTAssertEqual(
            try RequiredReason.violations(files: uptime, declared: ["NSPrivacyAccessedAPICategoryUserDefaults"]),
            ["\(plant):2: systemUptime needs \(boot) in PrivacyInfo.xcprivacy"]
        )
        let capacity = [(path: plant, text: "let free = url.volumeAvailableCapacity\n")]
        XCTAssertEqual(
            try RequiredReason.violations(files: capacity, declared: []),
            ["\(plant):1: volumeAvailableCapacity needs \(disk) in PrivacyInfo.xcprivacy"]
        )
        XCTAssertEqual(
            try RequiredReason.violations(
                files: [(path: plant, text: "let quiet = 1\n")],
                declared: ["NSPrivacyAccessedAPICategoryActiveKeyboards"]
            ),
            ["unused category NSPrivacyAccessedAPICategoryActiveKeyboards"]
        )
        XCTAssertEqual(
            try RequiredReason.violations(
                files: uptime,
                declared: ["NSPrivacyAccessedAPICategoryUserDefaults", boot]
            ),
            []
        )
    }

    func testRequiredReasonMapsEveryApiTokenToItsCategory() throws {
        let samples = [
            ("UserDefaults.standard", "UserDefaults", "NSPrivacyAccessedAPICategoryUserDefaults"),
            ("@AppStorage(\"k\") var mode = 1", "@AppStorage", "NSPrivacyAccessedAPICategoryUserDefaults"),
            ("ProcessInfo.processInfo.systemUptime", "systemUptime", "NSPrivacyAccessedAPICategorySystemBootTime"),
            ("mach_absolute_time()", "mach_absolute_time", "NSPrivacyAccessedAPICategorySystemBootTime"),
            ("attrs.creationDate", "creationDate", "NSPrivacyAccessedAPICategoryFileTimestamp"),
            (
                "try url.resourceValues(forKeys: [.creationDateKey])",
                "creationDate",
                "NSPrivacyAccessedAPICategoryFileTimestamp"
            ),
            ("attrs.modificationDate", "modificationDate", "NSPrivacyAccessedAPICategoryFileTimestamp"),
            (
                "attrs.contentModificationDate",
                "contentModificationDate",
                "NSPrivacyAccessedAPICategoryFileTimestamp"
            ),
            (
                "try manager.attributesOfItem(atPath: path)",
                "attributesOfItem",
                "NSPrivacyAccessedAPICategoryFileTimestamp"
            ),
            ("getattrlist(path, &attr, &buf, size, 0)", "getattrlist", "NSPrivacyAccessedAPICategoryFileTimestamp"),
            ("url.volumeAvailableCapacity", "volumeAvailableCapacity", "NSPrivacyAccessedAPICategoryDiskSpace"),
            (
                "try url.resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey])",
                "volumeAvailableCapacity",
                "NSPrivacyAccessedAPICategoryDiskSpace"
            ),
            ("attrs.systemFreeSize", "systemFreeSize", "NSPrivacyAccessedAPICategoryDiskSpace"),
            ("statfs(path, &stats)", "statfs", "NSPrivacyAccessedAPICategoryDiskSpace"),
            ("UITextInputMode.activeInputModes", "activeInputModes", "NSPrivacyAccessedAPICategoryActiveKeyboards"),
        ]
        for (text, token, category) in samples {
            let found = try RequiredReason.violations(
                files: [("apps/mobile/Monaco/Sample.swift", text)],
                declared: []
            )
            let message = "apps/mobile/Monaco/Sample.swift:1: \(token) needs \(category) in PrivacyInfo.xcprivacy"
            XCTAssertEqual(found, [message], text)
        }
    }
}
