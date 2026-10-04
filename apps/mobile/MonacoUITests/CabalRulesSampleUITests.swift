import XCTest

nonisolated final class CabalRulesSampleUITests: XCTestCase {
    private static let creatorID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private static let jordanID = "01890a5d-ac96-774b-bcce-b302099a8061"
    private static let rows = ["cabal-rules-join", "cabal-rules-voters", "cabal-rules-threshold", "cabal-rules-expiry"]

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
        let values = ["Anyone", "Every member", "Everyone agrees", "1 day"]
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
        let votersRow = element(app, "edit-cabal-voters")
        XCTAssertTrue(votersRow.waitForExistence(timeout: 10), "Edit cabal is pushed")
        XCTAssertTrue(votersRow.label.contains("Every member"), "the Voters row reads Every member")
        XCTAssertFalse(element(app, "edit-rule-voters").exists, "the editor has no Who votes segment")
        app.swipeUp()
        XCTAssertTrue(votersRow.isHittable, "the Voters row scrolls clear of the Save button")
        screenshot(app, "2-edit-cabal-voters-row")
        votersRow.tap()

        let pick = element(app, "voters-pick")
        XCTAssertTrue(pick.waitForExistence(timeout: 10), "Who votes is pushed")
        let save = element(app, "voters-save")
        XCTAssertFalse(save.isEnabled, "Save waits for a change")
        pick.tap()
        let jordan = element(app, "voters-member-\(Self.jordanID)")
        XCTAssertTrue(jordan.waitForExistence(timeout: 5), "picking voters lists the members")
        jordan.tap()
        XCTAssertTrue(save.isEnabled, "Save is enabled once the list changed")
        save.tap()
        XCTAssertTrue(app.staticTexts["Voters updated."].waitForExistence(timeout: 10), "the save toasts")
        screenshot(app, "2-voters-saved")
        XCTAssertFalse(save.isEnabled, "Save is disabled again after the save")

        let creator = element(app, "voters-member-\(Self.creatorID)")
        creator.tap()
        XCTAssertTrue(creator.isSelected, "the creator stays checked")
        XCTAssertTrue(creator.label.contains("Always votes"), "the creator's row says Always votes")
        screenshot(app, "3-creator-always-votes")

        app.navigationBars.buttons.element(boundBy: 0).tap()
        XCTAssertTrue(votersRow.waitForExistence(timeout: 5))
        XCTAssertTrue(
            votersRow.label.contains("Kai and Jordan"), "the editor reads the new list, got \(votersRow.label)")
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
