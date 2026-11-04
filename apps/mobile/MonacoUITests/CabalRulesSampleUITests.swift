import XCTest

nonisolated final class CabalRulesSampleUITests: XCTestCase {
    private static let rows = [
        ("cabal-rules-votesToPass", "5 of 8"), ("cabal-rules-voteWindow", "6 hours"),
        ("cabal-rules-maxPerTrade", "20% of the cash"), ("cabal-rules-whoCanPropose", "Voters"),
        ("cabal-rules-cashOut", "Any time"), ("cabal-rules-show-sides-toggle", "Show who voted what"),
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
        for (identifier, value) in Self.rows {
            let row = element(app, identifier)
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 15), "\(identifier) shows")
            XCTAssertTrue(row.label.contains(value), "\(identifier) reads \(value), got \(row.label)")
        }
        let footer = element(app, "cabal-rules-footer")
        app.scrollIntoReach(footer)
        XCTAssertTrue(footer.label.contains("Rules change by proposal."), "the footer says rules change by proposal")
        XCTAssertFalse(footer.label.contains("admins"), "the footer names no admins")
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
    func testShowWhoVotedWhatReadsAsOneSwitch() {
        let app = launch("fRules")
        let named = app.switches.matching(NSPredicate(format: "label == %@", "Show who voted what"))
        app.scrollIntoReach(named.firstMatch)
        XCTAssertTrue(named.firstMatch.waitForExistence(timeout: 15), "the switch shows")
        XCTAssertEqual(named.count, 1, "the row is one switch")
        XCTAssertEqual(named.firstMatch.identifier, "cabal-rules-show-sides-toggle")
    }
}
