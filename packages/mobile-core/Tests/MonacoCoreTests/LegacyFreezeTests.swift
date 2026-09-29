import XCTest
@testable import MonacoCore

/// The legacy hand-written API code only shrinks. `legacy-baseline.tsv` at the package root holds
/// today's counts; each rewire lowers its rows in the same PR that deletes the code.
final class LegacyFreezeTests: XCTestCase {

    private static let repoRoot = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()

    func testLegacyCode_matchesTheBaseline() throws {
        let baselineURL = Self.repoRoot.appendingPathComponent("packages/mobile-core/legacy-baseline.tsv")
        let baseline = try LegacyFreeze.parseBaseline(String(contentsOf: baselineURL, encoding: .utf8))

        let violations = try LegacyFreeze.violations(baseline: baseline, repoRoot: Self.repoRoot)

        XCTAssertEqual(violations, [], violations.joined(separator: "\n"))
    }

    func testPlantedURLRequestInANewFile_isCaught_andTheGeneratedClientIsExempt() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        try plant("let request = URLRequest(url: url)\n", at: "apps/mobile/Monaco/Features/Planted.swift", in: root)
        try plant("let request = URLRequest(url: url)\n", at: "packages/mobile-core/Sources/MonacoAPI/Client.swift", in: root)

        let violations = try LegacyFreeze.violations(baseline: [:], repoRoot: root)

        XCTAssertEqual(violations, ["legacy code grew: urlrequest apps/mobile/Monaco/Features/Planted.swift 0 -> 1"])
    }

    func testAppSources_reachNoExternalProductHost() throws {
        let clean = try ProductBoundaryScanner.featureSourcesAreClean(under: Self.repoRoot.appendingPathComponent("apps/mobile/Monaco"))

        XCTAssertTrue(clean)
    }

    func testPlantedForbiddenHost_isCaught() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        try plant("let quote = URL(string: \"https://api.jup.ag/swap/v1/quote\")\n", at: "Features/Quote.swift", in: root)

        let clean = try ProductBoundaryScanner.featureSourcesAreClean(under: root)

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
        case lines, urlrequest, timer, poll, groups_path

        var patterns: [String] {
            switch self {
            case .lines: return []
            case .urlrequest: return ["URLRequest("]
            case .timer: return ["Timer.publish"]
            case .poll: return ["pollWhileVisible(", "PollLoop.run("]
            case .groups_path: return ["\"/v1/groups"]
            }
        }
    }

    struct Row: Hashable {
        let metric: Metric
        let path: String
    }

    struct MalformedRow: Error, CustomStringConvertible {
        let line: String
        var description: String { "malformed baseline row: \(line)" }
    }

    static let scannedTrees = ["apps/mobile/Monaco", "packages/mobile-core/Sources"]
    static let exemptTree = "packages/mobile-core/Sources/MonacoAPI/"
    static let dtoTree = "apps/mobile/Monaco/API/DTOs/"
    static let lineCountedClients = [
        "apps/mobile/Monaco/API/MonacoAPIClient.swift",
        "packages/mobile-core/Sources/MonacoCore/MonacoAPIClient.swift",
    ]

    static func parseBaseline(_ text: String) throws -> [Row: Int] {
        var rows: [Row: Int] = [:]
        for line in text.split(separator: "\n") where !line.hasPrefix("#") {
            let fields = line.split(separator: "\t", omittingEmptySubsequences: false)
            guard fields.count == 3, let metric = Metric(rawValue: String(fields[0])), let count = Int(fields[2]) else {
                throw MalformedRow(line: String(line))
            }
            rows[Row(metric: metric, path: String(fields[1]))] = count
        }
        return rows
    }

    static func measure(repoRoot: URL) throws -> [Row: Int] {
        var counts: [Row: Int] = [:]
        for tree in scannedTrees {
            for file in ProductBoundaryScanner.sourceFiles(under: repoRoot.appendingPathComponent(tree)) {
                let path = relativePath(of: file, under: repoRoot)
                guard !path.hasPrefix(exemptTree) else { continue }
                let isLineCounted = path.hasPrefix(dtoTree) || lineCountedClients.contains(path)
                guard isLineCounted || file.pathExtension == "swift" else { continue }
                let text = try String(contentsOf: file, encoding: .utf8)
                if isLineCounted {
                    counts[Row(metric: .lines, path: path)] = text.utf8.filter { $0 == UInt8(ascii: "\n") }.count
                }
                guard file.pathExtension == "swift" else { continue }
                for metric in Metric.allCases where metric != .lines {
                    let matches = metric.patterns.reduce(0) { $0 + text.components(separatedBy: $1).count - 1 }
                    if matches > 0 {
                        counts[Row(metric: metric, path: path)] = matches
                    }
                }
            }
        }
        return counts
    }

    static func violations(baseline: [Row: Int], repoRoot: URL) throws -> [String] {
        let actual = try measure(repoRoot: repoRoot)
        var violations: [String] = []
        for (row, count) in actual {
            let allowed = baseline[row] ?? 0
            if count > allowed || baseline[row] == nil {
                violations.append("legacy code grew: \(row.metric.rawValue) \(row.path) \(allowed) -> \(count)")
            } else if count < allowed {
                violations.append("lower the baseline: \(row.metric.rawValue) \(row.path) \(allowed) -> \(count)")
            }
        }
        var droppedPaths: Set<String> = []
        for (row, allowed) in baseline where actual[row] == nil {
            if FileManager.default.fileExists(atPath: repoRoot.appendingPathComponent(row.path).path) {
                violations.append("lower the baseline: \(row.metric.rawValue) \(row.path) \(allowed) -> 0")
            } else {
                droppedPaths.insert(row.path)
            }
        }
        violations += droppedPaths.map { "drop the row: \($0)" }
        return violations.sorted()
    }

    private static func relativePath(of file: URL, under root: URL) -> String {
        let rootPath = root.resolvingSymlinksInPath().path + "/"
        let directory = file.deletingLastPathComponent().resolvingSymlinksInPath()
        return String(directory.appendingPathComponent(file.lastPathComponent).path.dropFirst(rootPath.count))
    }
}
