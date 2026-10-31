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
        for id in ["cabal-action-fund", "cabal-action-propose"] {
            XCTAssertTrue(element(app, id).isEnabled, "\(id) is enabled for a voter")
        }
        XCTAssertFalse(element(app, "cabal-action-propose-caption").exists, "a voter sees no caption")
        screenshot(app, "cabal-actions-voter")

        fund.tap()
        let fundScreen = element(app, "fund-cabal-view")
        XCTAssertTrue(fundScreen.waitForExistence(timeout: 10), "Fund opens the Fund this cabal screen")
        screenshot(app, "cabal-actions-cabal-action-fund")
        app.navigationBars.buttons.element(boundBy: 0).tap()
        XCTAssertTrue(fund.waitForExistence(timeout: 10), "back returns to the action row")
    }

    @MainActor
    func testProposeOpensTheActionSheetAndCancelClosesIt() {
        let app = launch(role: "voter")
        let propose = element(app, "cabal-action-propose")
        XCTAssertTrue(propose.waitForExistence(timeout: 15), "a voter sees the action row")
        propose.tap()
        let sheet = element(app, "propose-chooser-sheet")
        XCTAssertTrue(sheet.waitForExistence(timeout: 10), "Propose opens the action sheet")
        XCTAssertFalse(element(app, "propose-chooser").exists, "Propose does not push the full-screen chooser")
        XCTAssertTrue(element(app, "propose-kind-buy").exists, "the sheet offers Buy stock")
        screenshot(app, "cabal-actions-propose-sheet")
        element(app, "propose-chooser-cancel").tap()
        let closed = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: sheet)
        XCTAssertEqual(XCTWaiter().wait(for: [closed], timeout: 10), .completed, "Cancel closes the sheet")
        XCTAssertTrue(element(app, "cabal-action-propose").exists, "the cabal screen is still showing")
    }

    @MainActor
    func testANonVoterCannotPropose() {
        let app = launch(role: "nonvoter")
        let fund = element(app, "cabal-action-fund")
        XCTAssertTrue(fund.waitForExistence(timeout: 15), "a non-voting member sees the action row")
        XCTAssertTrue(fund.isEnabled, "Fund stays enabled")
        XCTAssertFalse(element(app, "cabal-action-propose").exists, "a non-voter sees no Propose button")
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
