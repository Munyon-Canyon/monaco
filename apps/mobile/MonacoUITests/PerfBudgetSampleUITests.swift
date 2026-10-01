import XCTest

private struct PerfBudgetReport: Decodable {
    let launchFirstFrameMilliseconds: Int?

    private enum CodingKeys: String, CodingKey {
        case launchFirstFrameMilliseconds = "launch_first_frame_ms"
    }
}

nonisolated final class PerfBudgetSampleUITests: XCTestCase {
    private static let budgets = URL(filePath: #filePath).deletingLastPathComponent()
        .appending(path: "perf-budgets.tsv")

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testLaunchToFirstFrameStaysUnderBudget() throws {
        let budget = try budget(screen: "Home", metric: "launch_first_frame_ms")
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoHomeSample", "populated", "-MonacoFrameStats"]
        var values: [Int] = []
        for _ in 1...5 {
            app.launch()
            values.append(try launchFirstFrameMilliseconds(app))
            app.terminate()
        }
        let median = values.sorted()[values.count / 2]
        let summary = "Home launch_first_frame_ms \(values), median \(median) ms, budget \(budget) ms"
        print(summary)
        XCTAssertLessThanOrEqual(median, budget, summary)
    }

    private func budget(screen: String, metric: String) throws -> Int {
        let key = "\(screen)\t\(metric)\t"
        let rows = try String(contentsOf: Self.budgets, encoding: .utf8).split(separator: "\n")
        let budget = rows.first { $0.hasPrefix(key) }.flatMap { Int($0.dropFirst(key.count)) }
        return try XCTUnwrap(budget, "no \(screen) \(metric) budget in \(Self.budgets.path)")
    }

    @MainActor
    private func launchFirstFrameMilliseconds(_ app: XCUIApplication) throws -> Int {
        let report = app.otherElements["frame-stats-report"]
        XCTAssertTrue(report.waitForExistence(timeout: 30), "Home never showed the frame-stats-report element")
        let reported = NSPredicate(format: "NOT (value CONTAINS %@)", "\"launch_first_frame_ms\":null")
        _ = XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: reported, object: report)], timeout: 15)
        let json = try XCTUnwrap(report.value as? String, "frame-stats-report has no value")
        let decoded = try JSONDecoder().decode(PerfBudgetReport.self, from: Data(json.utf8))
        return try XCTUnwrap(
            decoded.launchFirstFrameMilliseconds,
            "launch_first_frame_ms is still null in \(json)"
        )
    }
}
