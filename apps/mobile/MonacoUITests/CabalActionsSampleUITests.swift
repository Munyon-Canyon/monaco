import XCTest

nonisolated final class CabalActionsSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(role: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-cabalActionsHarness", role]
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
    func testAVoterOpensEachActionsPlaceholder() {
        let app = launch(role: "voter")
        let fund = element(app, "cabal-action-fund")
        XCTAssertTrue(fund.waitForExistence(timeout: 15), "a voter sees the action row")
        for id in ["cabal-action-fund", "cabal-action-propose", "cabal-action-cash-out", "cabal-action-chat"] {
            XCTAssertTrue(element(app, id).isEnabled, "\(id) is enabled for a voter")
        }
        XCTAssertFalse(element(app, "cabal-action-propose-caption").exists, "a voter sees no caption")
        screenshot(app, "cabal-actions-voter")

        let placeholders = [
            ("cabal-action-fund", "Fund this cabal"),
            ("cabal-action-propose", "Propose"),
            ("cabal-action-cash-out", "Cash out"),
            ("cabal-action-chat", "Chat"),
        ]
        for (id, screen) in placeholders {
            element(app, id).tap()
            let placeholder = app.staticTexts["\(screen) isn't on the new backend yet."]
            XCTAssertTrue(placeholder.waitForExistence(timeout: 10), "\(id) opens the \(screen) placeholder")
            screenshot(app, "cabal-actions-\(id)")
            app.navigationBars.buttons.element(boundBy: 0).tap()
            XCTAssertTrue(element(app, id).waitForExistence(timeout: 10), "back returns to the action row")
        }
    }

    @MainActor
    func testANonVoterCannotPropose() {
        let app = launch(role: "nonvoter")
        let propose = element(app, "cabal-action-propose")
        XCTAssertTrue(propose.waitForExistence(timeout: 15), "a non-voting member sees the action row")
        XCTAssertFalse(propose.isEnabled, "Propose is disabled")
        XCTAssertTrue(element(app, "cabal-action-fund").isEnabled, "Fund stays enabled")
        let caption = element(app, "cabal-action-propose-caption")
        XCTAssertTrue(caption.exists, "the caption explains why")
        XCTAssertEqual(caption.label, "Only voters can propose")
        screenshot(app, "cabal-actions-nonvoter")
    }

    @MainActor
    func testAnOutsiderSeesNoActionRow() {
        let app = launch(role: "outsider")
        XCTAssertTrue(app.navigationBars["QA pot"].waitForExistence(timeout: 15), "the harness is on screen")
        XCTAssertFalse(element(app, "cabal-action-fund").waitForExistence(timeout: 3), "an outsider sees no action row")
        screenshot(app, "cabal-actions-outsider")
    }
}
