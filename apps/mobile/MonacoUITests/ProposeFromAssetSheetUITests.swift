import XCTest

nonisolated final class ProposeFromAssetSheetUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoProposeSample", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func cabalRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(
            NSPredicate(
                format: "identifier BEGINSWITH 'propose-pick-cabal-' AND NOT identifier IN %@",
                ["propose-pick-cabal-question", "propose-pick-cabal-sheet"]))
    }

    @MainActor
    func testSeveralCabalsAskInAHalfSheet() throws {
        let app = launch("pickCabal")
        let title = element(app, "propose-pick-cabal-sheet")
        XCTAssertTrue(title.waitForExistence(timeout: 30), "the cabal picker never opened as a sheet")
        XCTAssertEqual(title.label, "Choose a cabal")
        XCTAssertTrue(element(app, "propose-pick-cabal-question").exists, "the picker should ask which cabal")
        XCTAssertGreaterThanOrEqual(cabalRows(app).count, 2, "the picker lists fewer than two cabals")
        let screen = app.windows.firstMatch.frame.height
        XCTAssertGreaterThan(title.frame.minY, screen * 0.35, "the sheet should stay about half the screen")
        app.swipeUp()
        XCTAssertGreaterThan(title.frame.minY, screen * 0.35, "the sheet should never expand to full screen")
    }

    @MainActor
    func testPickingACabalClosesTheSheetAndOpensTheAmountStep() throws {
        let app = launch("pickCabal")
        XCTAssertTrue(element(app, "propose-pick-cabal-sheet").waitForExistence(timeout: 30), "no picker sheet")
        let row = cabalRows(app).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10), "no cabal row")
        row.tap()
        XCTAssertTrue(app.navigationBars["Buy Alphabet"].waitForExistence(timeout: 15), "no amount step after a pick")
        XCTAssertFalse(element(app, "propose-pick-cabal-sheet").exists, "the sheet should be closed")
    }
}
