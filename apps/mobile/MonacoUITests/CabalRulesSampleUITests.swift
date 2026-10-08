import XCTest

nonisolated final class CabalRulesSampleUITests: XCTestCase {
    private static let creatorID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private static let jordanID = "01890a5d-ac96-774b-bcce-b302099a8061"
    private static let rows = [
        "cabal-rules-name", "cabal-rules-join", "cabal-rules-voters", "cabal-rules-threshold", "cabal-rules-expiry",
    ]

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(role: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-cabalRulesHarness", role]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let shot = app.screenshot()
        let attachment = XCTAttachment(screenshot: shot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        if let directory = ProcessInfo.processInfo.environment["MONACO_QA_SHOTS"] {
            try? shot.pngRepresentation.write(to: URL(fileURLWithPath: directory).appendingPathComponent("\(name).png"))
        }
    }

    @MainActor
    private func assertRuleValues(_ app: XCUIApplication) {
        let values = ["QA pot", "Approval required", "Every member", "Everyone agrees", "1 day"]
        for (identifier, value) in zip(Self.rows, values) {
            let row = element(app, identifier)
            XCTAssertTrue(row.waitForExistence(timeout: 15), "\(identifier) shows")
            XCTAssertTrue(row.label.contains(value), "\(identifier) reads \(value), got \(row.label)")
        }
    }

    @MainActor
    func testTheCreatorPicksAVoterFromTheRules() {
        let app = launch(role: "creator")
        XCTAssertTrue(app.navigationBars["Cabal details"].waitForExistence(timeout: 15))
        assertRuleValues(app)
        for identifier in Self.rows {
            XCTAssertTrue(app.buttons[identifier].exists, "the creator can open \(identifier)")
        }
        screenshot(app, "1-rules-creator")

        app.buttons["cabal-rules-voters"].tap()
        let voters = element(app, "edit-rule-voters")
        XCTAssertTrue(voters.waitForExistence(timeout: 10), "Cabal settings is pushed with a Who votes row")
        XCTAssertFalse(element(app, "edit-cabal-voters").exists, "Everyone hides the member list")
        let save = element(app, "edit-cabal-save")
        XCTAssertFalse(save.isEnabled, "Save waits for a change")
        voters.buttons["People I pick"].tap()
        let jordan = element(app, "voters-member-\(Self.jordanID)")
        XCTAssertTrue(jordan.waitForExistence(timeout: 5), "picking voters lists the members")
        jordan.tap()
        XCTAssertTrue(save.isEnabled, "Save is enabled once the list changed")
        let creator = element(app, "voters-member-\(Self.creatorID)")
        creator.tap()
        XCTAssertTrue(creator.isSelected, "the creator stays checked")
        XCTAssertTrue(creator.label.contains("Always votes"), "the creator's row says Always votes")
        screenshot(app, "2-voters-picked")
        save.tap()
        XCTAssertTrue(app.staticTexts["Cabal updated."].waitForExistence(timeout: 10), "the save toasts")
        screenshot(app, "2-voters-saved")
        XCTAssertFalse(save.isEnabled, "Save is disabled again after the save")

        app.navigationBars.buttons.element(boundBy: 0).tap()
        let rulesVoters = element(app, "cabal-rules-voters")
        XCTAssertTrue(rulesVoters.waitForExistence(timeout: 5))
        XCTAssertTrue(
            rulesVoters.label.contains("Kai and Jordan"), "Who votes names the creator first, got \(rulesVoters.label)")
        screenshot(app, "3-rules-after-save")
    }

    @MainActor
    func testAMemberReadsTheRulesWithoutChevrons() {
        let app = launch(role: "member")
        XCTAssertTrue(app.navigationBars["Cabal details"].waitForExistence(timeout: 15))
        assertRuleValues(app)
        for identifier in Self.rows {
            XCTAssertFalse(app.buttons[identifier].exists, "a member cannot open \(identifier)")
        }
        screenshot(app, "4-rules-member")
    }
}
