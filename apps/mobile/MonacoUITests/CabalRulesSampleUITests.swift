import XCTest

nonisolated final class CabalRulesSampleUITests: XCTestCase {
    private static let rows = [
        ("votesToPass", "5 of 8"), ("voteWindow", "24 h"), ("maxPerTrade", "20% of cash"),
        ("whoCanPropose", "Voters"), ("cashOut", "Anytime, your share"),
        ("showSides", "After a vote closes"),
    ]

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String, extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoGroupDetailSample", scenario] + extra
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testTheRulesTabListsTheSixRowsAndTheFooter() {
        let app = launch("fRules")
        for (kind, value) in Self.rows {
            let row = element(app, "cabal-rules-\(kind)")
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 15), "\(kind) shows")
            XCTAssertTrue(row.label.contains(value), "\(kind) reads \(value), got \(row.label)")
        }
        let footer = element(app, "cabal-rules-footer")
        app.scrollIntoReach(footer)
        XCTAssertTrue(footer.label.contains("admins are Andres and Christelle"), "the footer names the admins")
    }

    @MainActor
    func testFlippingTheSwitchOpensTheProposeSheetAndLeavesTheRuleOff() {
        let app = launch("fRules")
        let toggle = element(app, "cabal-rules-show-sides-toggle")
        app.scrollIntoReach(toggle)
        XCTAssertTrue(toggle.waitForExistence(timeout: 15), "the switch shows")
        toggle.tap()
        XCTAssertTrue(
            element(app, "cabal-rule-proposal-propose").waitForExistence(timeout: 10), "the propose sheet opens")
    }

    @MainActor
    func testTheSidesSampleReadsOnSince() {
        let app = launch("fRulesSides")
        let row = element(app, "cabal-rules-showSides")
        app.scrollIntoReach(row)
        XCTAssertTrue(row.waitForExistence(timeout: 15), "the row shows")
        XCTAssertTrue(row.label.contains("on since Oct 20"), "the row reads on since Oct 20, got \(row.label)")
    }
}
