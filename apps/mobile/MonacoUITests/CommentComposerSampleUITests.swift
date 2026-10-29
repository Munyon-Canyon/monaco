import XCTest

nonisolated final class CommentComposerSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-composerHarness", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testTheDockedBarShowsThePlusTheFieldWithGifInsideAndADimSend() {
        let app = launch("docked")
        let field = app.textFields["comment-composer-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 15), "no composer field")
        XCTAssertEqual(field.placeholderValue, "Say something")
        let gif = app.buttons["comment-composer-gif"]
        XCTAssertFalse(app.buttons["comment-composer-plus"].exists, "the plus shows with no menu rows")
        XCTAssertTrue(gif.exists, "no GIF")
        XCTAssertGreaterThan(gif.frame.minX, field.frame.minX, "the GIF button is not inside the field")
        XCTAssertFalse(app.buttons["comment-composer-send"].isEnabled, "an empty field can send")
        XCTAssertFalse(app.otherElements["reply-sheet"].exists, "a sheet opened by itself")
    }

    @MainActor
    func testTheCommentSheetQuotesTheCommentAndFocusesTheField() {
        let app = launch("sheetComment")
        XCTAssertTrue(element(app, "reply-sheet-title").waitForExistence(timeout: 15), "no sheet")
        XCTAssertTrue(app.staticTexts["Reply to Mia"].exists, "no title")
        XCTAssertTrue(element(app, "reply-sheet-quote").exists, "no quote")
        XCTAssertTrue(app.buttons["comment-composer-gif"].exists, "no GIF inside the field")
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 10), "the field is not focused")
        XCTAssertTrue(app.buttons["reply-sheet-close"].exists, "no close")
        let fields = app.textFields.matching(identifier: "comment-composer-field")
        XCTAssertEqual(fields.count, 1, "the docked bar is still there")
    }

    @MainActor
    func testClosingTheSheetBringsTheDockedBarBack() {
        let app = launch("sheetComment")
        let close = app.buttons["reply-sheet-close"]
        XCTAssertTrue(close.waitForExistence(timeout: 15), "no close")
        close.tap()
        XCTAssertTrue(app.textFields["comment-composer-field"].waitForExistence(timeout: 10), "no docked bar")
        XCTAssertFalse(app.keyboards.firstMatch.exists, "the docked bar came back focused")
    }

    @MainActor
    func testReplyingToAPostQuotesItFocusesTheFieldAndTheToastSaysReplied() {
        let app = launch("sheetPost")
        XCTAssertTrue(element(app, "reply-sheet-quote").waitForExistence(timeout: 15), "no quoted post")
        XCTAssertTrue(app.staticTexts["Reply to Logan"].exists, "no title")
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 10), "the field is not focused")
        let field = app.textFields["comment-composer-field"]
        field.typeText("Clean exit.")
        app.buttons["comment-composer-send"].tap()
        let toast = element(app, "monaco-toast-banner")
        XCTAssertTrue(toast.waitForExistence(timeout: 10), "no Replied toast")
        XCTAssertTrue(toast.label.contains("Replied"), toast.label)
        XCTAssertTrue(element(app, "reply-sheet-quote").waitForNonExistence(timeout: 10), "the sheet stayed open")
    }

    @MainActor
    func testAFailedSendKeepsTheSheetTheTextAndShowsTheNotice() {
        let app = launch("failed")
        let field = app.textFields["comment-composer-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 15), "no composer field")
        field.tap()
        field.typeText("Count me in.")
        app.buttons["comment-composer-send"].tap()
        XCTAssertTrue(
            app.staticTexts["Your reply wasn’t sent. Your text is still here."].waitForExistence(timeout: 10),
            "no failure notice")
        XCTAssertEqual(field.value as? String, "Count me in.", "the draft was lost")
    }
}
