import XCTest

nonisolated final class ProposeReviewSendingUITests: XCTestCase {
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
    private func waitForSending(_ app: XCUIApplication) {
        let send = element(app, "propose-review-send")
        let sending = NSPredicate(format: "label CONTAINS 'Sending'")
        let started = XCTNSPredicateExpectation(predicate: sending, object: send)
        XCTAssertEqual(XCTWaiter().wait(for: [started], timeout: 10), .completed, "the send never started")
    }

    @MainActor
    func testChangeCabalIsDisabledWhileSending() throws {
        let app = launch("reviewSubmitting")
        let change = element(app, "propose-review-change-cabal")
        XCTAssertTrue(change.waitForExistence(timeout: 30), "the review never drew its Change button")
        waitForSending(app)
        XCTAssertFalse(change.isEnabled, "Change stayed tappable while the proposal was sending")
        XCTAssertFalse(element(app, "propose-review-send").isEnabled, "Send stayed tappable while sending")
    }

    @MainActor
    func testCloseIsGoneAndTheSheetStaysWhileSending() throws {
        let app = launch("reviewSubmitting")
        let sheet = element(app, "propose-review-sheet")
        XCTAssertTrue(sheet.waitForExistence(timeout: 30), "the review sheet never opened")
        waitForSending(app)
        XCTAssertFalse(app.buttons["Close"].exists, "the X stayed on the sheet while it was sending")
        app.swipeDown()
        XCTAssertTrue(sheet.exists, "the sheet dismissed mid-send")
    }
}
