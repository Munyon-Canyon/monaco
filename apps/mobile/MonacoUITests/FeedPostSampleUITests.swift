import XCTest

nonisolated final class FeedPostSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ mode: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-feedPostHarness", mode]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func matching(_ app: XCUIApplication, prefix: String, contains text: String = "") -> XCUIElement {
        let format = text.isEmpty ? "identifier BEGINSWITH %@" : "identifier BEGINSWITH %@ AND identifier CONTAINS %@"
        let predicate = text.isEmpty ? NSPredicate(format: format, prefix) : NSPredicate(format: format, prefix, text)
        return app.descendants(matching: .any).matching(predicate).firstMatch
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testTheLoganPostShowsItsBarReactionsRepliesAndComposer() {
        let app = launch("logan")
        XCTAssertTrue(element(app, "comment-row-aapl-nora").waitForExistence(timeout: 15), "Nora's reply is drawn")
        XCTAssertTrue(app.staticTexts["Post"].exists, "the inline bar reads Post")
        let post = matching(app, prefix: "feed-cell-")
        XCTAssertTrue(post.exists, "Logan's sale is drawn as Feed A draws it")
        screenshot(app, "post-logan")
        XCTAssertTrue(post.label.contains("Logan"), "the post is Logan's, not \(post.label)")
        XCTAssertTrue(
            matching(app, prefix: "reaction-chip-feed", contains: "1F4B0").exists, "the 💰 chip is on the post")
        XCTAssertTrue(
            matching(app, prefix: "reaction-chip-feed", contains: "1F64C").exists, "the 🙌 chip is on the post")
        XCTAssertTrue(matching(app, prefix: "feed-reply-").exists, "the reply control is on the post")
        XCTAssertTrue(app.staticTexts["Replies"].exists, "the header reads Replies")
        XCTAssertTrue(app.staticTexts["5"].exists, "the header carries the count")
        XCTAssertTrue(app.buttons["View 1 reply"].exists, "Nora's fold reads View 1 reply")
        XCTAssertTrue(app.textFields["comment-composer-field"].exists, "the composer is docked")
        XCTAssertTrue(app.buttons["comment-composer-send"].exists, "the composer has Send")
        screenshot(app, "post-logan")
    }

    @MainActor
    func testThePostsReplyControlFocusesTheComposerAndOpensNoSheet() {
        let app = launch("logan")
        let control = matching(app, prefix: "feed-reply-")
        XCTAssertTrue(control.waitForExistence(timeout: 15), "no reply control")

        control.tap()

        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 10), "the keyboard did not come up")
        XCTAssertFalse(app.buttons["reply-sheet-close"].exists, "the reply control opened the sheet")
        screenshot(app, "post-reply-focused")
    }

    @MainActor
    func testARowsReplyOpensTheSheetQuotingIt() {
        let app = launch("logan")
        let reply = element(app, "comment-reply-aapl-nora")
        XCTAssertTrue(reply.waitForExistence(timeout: 15), "no Reply on Nora's row")

        reply.tap()

        XCTAssertTrue(app.staticTexts["Reply to Nora"].waitForExistence(timeout: 5), "no reply sheet")
    }

    @MainActor
    func testTheEndOfTheRepliesRestsSamsReplyFortyAboveTheComposer() {
        let app = launch("end")
        let sam = element(app, "comment-row-aapl-sam")
        let field = app.textFields["comment-composer-field"]
        XCTAssertTrue(sam.waitForExistence(timeout: 15), "Sam's reply is drawn")
        XCTAssertTrue(field.waitForExistence(timeout: 5), "the composer is docked")
        let settled = NSPredicate(format: "isHittable == true")
        expectation(for: settled, evaluatedWith: sam)
        waitForExpectations(timeout: 10)
        Thread.sleep(forTimeInterval: 2)

        let gap = field.frame.minY - sam.frame.maxY

        XCTAssertGreaterThanOrEqual(gap, 40, "Sam's reply sits closer than 40 to the composer")
        XCTAssertLessThan(gap, 64, "Sam's reply is far above the composer")
        screenshot(app, "post-end")
    }

    @MainActor
    func testAnUnavailablePostStillGoesBackToTheFeed() {
        let app = launch("unavailable")
        let back = app.buttons["Back to feed"]
        XCTAssertTrue(back.waitForExistence(timeout: 15), "no Back to feed")
        XCTAssertFalse(app.buttons["comment-composer-send"].exists, "the composer is still there")
    }
}
