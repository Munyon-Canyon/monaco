import XCTest

@testable import MonacoCore

final class LegacyFreezeTests: XCTestCase {

    private static let repoRoot = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()

    func testLegacyCode_staysInsideTheFrozenList() throws {
        let listURL = Self.repoRoot.appendingPathComponent("packages/mobile-core/legacy-baseline.tsv")
        let listed = try LegacyFreeze.parseBaseline(String(contentsOf: listURL, encoding: .utf8))

        let violations = try LegacyFreeze.violations(listed: listed, repoRoot: Self.repoRoot)

        XCTAssertEqual(violations, [], violations.joined(separator: "\n"))
    }

    func testPlantedURLRequestInANewFile_isCaught_andTheGeneratedClientIsExempt() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        try plant("let request = URLRequest(url: url)\n", at: "apps/mobile/Monaco/Features/Planted.swift", in: root)
        try plant(
            "let request = URLRequest(url: url)\n", at: "packages/mobile-core/Sources/MonacoAPI/Client.swift", in: root)

        let violations = try LegacyFreeze.violations(listed: [], repoRoot: root)

        XCTAssertEqual(
            violations, ["legacy pattern outside the list: urlrequest apps/mobile/Monaco/Features/Planted.swift"])
    }

    func testPlantedFileUnderAPI_isCaught() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        try plant("struct NewDTO: Decodable {}\n", at: "apps/mobile/Monaco/API/DTOs/NewDTO.swift", in: root)

        let violations = try LegacyFreeze.violations(listed: [], repoRoot: root)

        XCTAssertEqual(violations, ["new file under API: apps/mobile/Monaco/API/DTOs/NewDTO.swift"])
    }

    func testGoneAndCleanListedFiles_areIgnored() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        try plant("struct Clean {}\n", at: "apps/mobile/Monaco/API/Clean.swift", in: root)

        let violations = try LegacyFreeze.violations(
            listed: ["apps/mobile/Monaco/API/Clean.swift", "apps/mobile/Monaco/API/Gone.swift"], repoRoot: root)

        XCTAssertEqual(violations, [])
    }

    func testAppSources_reachNoExternalProductHost() throws {
        let clean = try FeatureHosts.areClean(under: Self.repoRoot.appendingPathComponent("apps/mobile/Monaco"))

        XCTAssertTrue(clean)
    }

    func testPlantedForbiddenHost_isCaught() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        try plant(
            "let quote = URL(string: \"https://api.jup.ag/swap/v1/quote\")\n", at: "Features/Quote.swift", in: root)

        let clean = try FeatureHosts.areClean(under: root)

        XCTAssertFalse(clean)
    }

    private func plant(_ text: String, at path: String, in root: URL) throws {
        let file = root.appendingPathComponent(path)
        try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try text.write(to: file, atomically: true, encoding: .utf8)
    }
}

private enum LegacyFreeze {
    enum Metric: String, CaseIterable {
        case urlrequest, timer, poll
        case groupsPath = "groups_path"

        var patterns: [String] {
            switch self {
            case .urlrequest: return ["URLRequest("]
            case .timer: return ["Timer.publish"]
            case .poll: return ["pollWhileVisible(", "PollLoop.run("]
            case .groupsPath: return ["\"/v1/groups"]
            }
        }
    }

    struct MalformedRow: Error, CustomStringConvertible {
        let line: String
        var description: String { "malformed baseline row: \(line)" }
    }

    static let scannedTrees = ["apps/mobile/Monaco", "packages/mobile-core/Sources"]
    static let exemptTree = "packages/mobile-core/Sources/MonacoAPI/"
    static let apiTree = "apps/mobile/Monaco/API/"

    static func parseBaseline(_ text: String) throws -> Set<String> {
        var paths: Set<String> = []
        let body = text.hasSuffix("\n") ? text.dropLast() : Substring(text)
        for line in body.split(separator: "\n", omittingEmptySubsequences: false) {
            guard !line.isEmpty, !line.contains("\t") else {
                throw MalformedRow(line: String(line))
            }
            paths.insert(String(line))
        }
        return paths
    }

    static func violations(listed: Set<String>, repoRoot: URL) throws -> [String] {
        var violations: [String] = []
        for tree in scannedTrees {
            for file in SourceWalk.files(under: repoRoot.appendingPathComponent(tree)) {
                let path = relativePath(of: file, under: repoRoot)
                guard !path.hasPrefix(exemptTree), !listed.contains(path) else { continue }
                if path.hasPrefix(apiTree) {
                    violations.append("new file under API: \(path)")
                }
                guard file.pathExtension == "swift" else { continue }
                let text = try String(contentsOf: file, encoding: .utf8)
                for metric in Metric.allCases where metric.patterns.contains(where: text.contains) {
                    violations.append("legacy pattern outside the list: \(metric.rawValue) \(path)")
                }
            }
        }
        return violations.sorted()
    }

    private static func relativePath(of file: URL, under root: URL) -> String {
        let rootPath = root.resolvingSymlinksInPath().path + "/"
        let directory = file.deletingLastPathComponent().resolvingSymlinksInPath()
        return String(directory.appendingPathComponent(file.lastPathComponent).path.dropFirst(rootPath.count))
    }
}

private enum SourceWalk {
    static func files(under directory: URL) -> [URL] {
        guard
            let walk = FileManager.default.enumerator(
                at: directory, includingPropertiesForKeys: [.isRegularFileKey])
        else {
            return []
        }
        return
            walk
            .compactMap { $0 as? URL }
            .filter { (try? $0.resourceValues(forKeys: [.isRegularFileKey]).isRegularFile) == true }
            .sorted { $0.path < $1.path }
    }
}

private enum FeatureHosts {
    static let fragments = [
        "api.xstocks.fi",
        "jup.ag",
        "hermes.pyth.network",
        "pyth.network",
        "mainnet-beta.solana.com",
        "solana-mainnet",
    ]

    static func areClean(under directory: URL) throws -> Bool {
        try SourceWalk.files(under: directory)
            .filter { $0.pathExtension == "swift" }
            .allSatisfy { file in
                let text = try String(contentsOf: file, encoding: .utf8).lowercased()
                return !fragments.contains { text.contains($0) }
            }
    }
}
