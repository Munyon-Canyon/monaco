import XCTest

nonisolated final class CabalEditSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(role: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-cabalEditHarness", role]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testTheCreatorRenamesTheCabalAndChangesARule() {
        let app = launch(role: "creator")
        let row = element(app, "cabal-edit-row")
        XCTAssertTrue(row.waitForExistence(timeout: 15), "the creator sees Edit cabal")
        row.tap()

        let name = element(app, "edit-cabal-name")
        XCTAssertTrue(name.waitForExistence(timeout: 10), "Edit cabal is pushed")
        XCTAssertEqual(name.value as? String, "QA pot", "the name is prefilled")
        let save = element(app, "edit-cabal-save")
        XCTAssertFalse(save.isEnabled, "Save is disabled while nothing changed")
        XCTAssertTrue(app.staticTexts["Rule changes apply to new proposals. Open votes keep their rules."].exists)
        screenshot(app, "edit-cabal-prefilled")

        name.tap()
        name.typeText(" 2")
        let majority = element(app, "edit-rule-threshold").buttons["Majority"]
        XCTAssertTrue(majority.waitForExistence(timeout: 5), "the To pass picker offers Majority")
        majority.tap()
        XCTAssertTrue(save.isEnabled, "Save is enabled once something changed")
        save.tap()

        XCTAssertTrue(app.staticTexts["Cabal updated."].waitForExistence(timeout: 10), "the save toasts")
        screenshot(app, "edit-cabal-saved")
        XCTAssertFalse(save.isEnabled, "Save is disabled again after the save")
    }

    @MainActor
    func testAMemberWhoDidNotCreateTheCabalSeesNoEditRow() {
        let app = launch(role: "member")
        XCTAssertTrue(app.navigationBars["Details"].waitForExistence(timeout: 15))
        XCTAssertFalse(element(app, "cabal-edit-row").waitForExistence(timeout: 3), "a member gets no Edit cabal row")
        screenshot(app, "edit-cabal-member")
    }
}
