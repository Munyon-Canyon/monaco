import XCTest

nonisolated final class CommentActionsSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-commentThreadHarness", "logan"]
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
    private func longPress(_ app: XCUIApplication, _ id: String) {
        let body = element(app, "comment-body-\(id)")
        XCTAssertTrue(body.waitForExistence(timeout: 15), "\(id) is drawn")
        body.press(forDuration: 1.0)
        XCTAssertTrue(
            element(app, "comment-actions-\(id)").waitForExistence(timeout: 5), "long-press opens the actions")
    }

    @MainActor
    func testLongPressingSomeoneElsesCommentOffersReactReplyCopyAndReportButNotDelete() {
        let app = launch()
        longPress(app, "aapl-nora")
        screenshot(app, "actions-nora")
        XCTAssertTrue(element(app, "reaction-tray-1F389").exists, "the tray is above the row")
        XCTAssertTrue(element(app, "comment-action-reply").exists, "Reply is offered")
        XCTAssertTrue(element(app, "comment-action-copy").exists, "Copy is offered")
        XCTAssertTrue(element(app, "comment-action-report").exists, "Report is offered")
        XCTAssertFalse(element(app, "comment-action-delete").exists, "Delete is not offered on others' comments")
    }

    @MainActor
    func testTappingATrayEmojiReactsAndClosesTheActions() {
        let app = launch()
        longPress(app, "aapl-nora")
        element(app, "reaction-tray-1F389").tap()
        let chip = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'party popper, 1 reaction'")).firstMatch
        XCTAssertTrue(chip.waitForExistence(timeout: 10), "a party popper chip joins Nora's row")
        XCTAssertTrue(chip.label.contains("yours"), "the chip is the viewer's, so selected")
        XCTAssertFalse(element(app, "comment-actions-aapl-nora").exists, "the actions closed")
    }

    @MainActor
    func testLongPressingYourOwnCommentOffersDeleteButNotReport() {
        let app = launch()
        XCTAssertTrue(element(app, "comment-fold-aapl-nora").waitForExistence(timeout: 15), "Nora's fold is drawn")
        element(app, "comment-fold-aapl-nora").tap()
        longPress(app, "aapl-logan")
        XCTAssertTrue(element(app, "comment-action-delete").exists, "Delete is offered on your own comment")
        XCTAssertFalse(element(app, "comment-action-report").exists, "Report is not offered on your own comment")
    }

    @MainActor
    func testCopyToastsCopied() {
        let app = launch()
        longPress(app, "aapl-nora")
        element(app, "comment-action-copy").tap()
        XCTAssertTrue(app.staticTexts["Copied"].waitForExistence(timeout: 5), "Copy toasts Copied")
    }
}
