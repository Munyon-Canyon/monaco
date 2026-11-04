import XCTest

nonisolated final class JourneyWaitTests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testEarlyStopMessageNamesBothIdentifiers() {
        let message = JourneyDiagnostics.message(
            for: .deadEnd(id: "proposal-load-failed", label: "Couldn't load this vote.", after: 2.43),
            step: "S2.3", target: "cabal-open-proposal-7f3", timeout: 15)
        XCTAssertEqual(
            message,
            "S2.3: waited for cabal-open-proposal-7f3 but the screen shows proposal-load-failed (\"Couldn't load this vote.\") after 2.4 s"
        )
    }

    @MainActor
    func testTimeoutMessageListsTenNearestAndTheTitle() {
        let ids = (0..<12).map { "other-thing-\($0)" } + ["proposal-stat-rows", "proposal-stat-ro"]
        let message = JourneyDiagnostics.message(
            for: .timedOut(nearest: ids, title: "Buy GOOGL"),
            step: "S1.2", target: "proposal-stat-row", timeout: 15)
        XCTAssertTrue(
            message.hasPrefix(
                "S1.2: proposal-stat-row did not show within 15 s; on screen: proposal-stat-rows, proposal-stat-ro, "),
            message)
        XCTAssertTrue(message.hasSuffix(" … (title \"Buy GOOGL\")"), message)
        let listed = message.components(separatedBy: "on screen: ")[1].components(separatedBy: " …")[0]
        XCTAssertEqual(listed.components(separatedBy: ", ").count, 10)
    }

    @MainActor
    func testTimeoutMessageOmitsTheTitleClauseWithoutOne() {
        let message = JourneyDiagnostics.message(
            for: .timedOut(nearest: ["a"], title: nil), step: "S1", target: "b", timeout: 5)
        XCTAssertFalse(message.contains("title"), message)
    }

    @MainActor
    func testAllowingSuppressesADeadEnd() {
        let ids = ["home", "proposal-load-failed", "cabal-error"]
        XCTAssertEqual(JourneyDiagnostics.deadEndID(in: ids, allowing: []), "proposal-load-failed")
        XCTAssertEqual(
            JourneyDiagnostics.deadEndID(in: ids, allowing: ["proposal-load-failed"]), "cabal-error")
        XCTAssertNil(JourneyDiagnostics.deadEndID(in: ids, allowing: ["proposal-load-failed", "cabal-error"]))
        XCTAssertNil(JourneyDiagnostics.deadEndID(in: ["failed-banner", "errors"], allowing: []))
    }

    @MainActor
    func testLevenshtein() {
        XCTAssertEqual(JourneyDiagnostics.levenshtein("kitten", "sitting"), 3)
        XCTAssertEqual(JourneyDiagnostics.levenshtein("", "abc"), 3)
    }

    @MainActor
    func testWaitStopsAtAFailedIdentifierInsteadOfRunningOutTheClock() {
        let app = XCUIApplication()
        app.launchArguments = ["-composerHarness", "failed"]
        app.launch()
        let field = app.textFields["comment-composer-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 15), "no composer field")
        field.tap()
        field.typeText("Count me in.")
        app.buttons["comment-composer-send"].tap()
        let missing = app.descendants(matching: .any).matching(identifier: "no-such-element").firstMatch
        let start = Date()
        let result = JourneyDiagnostics.waitResult(for: missing, in: app, timeout: 15)
        XCTAssertLessThan(Date().timeIntervalSince(start), 3, "the wait ran out the clock")
        guard case .deadEnd(let id, _, _) = result else { return XCTFail("expected a dead end, got \(result)") }
        XCTAssertEqual(id, "comment-composer-failed")
        let message = JourneyDiagnostics.message(for: result, step: "T", target: "no-such-element", timeout: 15)
        XCTAssertTrue(message.contains("no-such-element") && message.contains("comment-composer-failed"), message)
    }
}
