import XCTest

nonisolated final class CommentThreadSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-commentThreadHarness", scenario]
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
    func testTheHeaderReadsRepliesWithTheCountAndTheFoldReadsViewOneReply() {
        let app = launch("logan")
        XCTAssertTrue(element(app, "comment-row-aapl-nora").waitForExistence(timeout: 15), "Nora's reply is drawn")
        XCTAssertTrue(app.staticTexts["Replies"].exists, "the header reads Replies")
        XCTAssertTrue(app.staticTexts["5"].exists, "the header carries the count")
        XCTAssertFalse(app.staticTexts["Discussion"].exists, "Discussion is gone")
        XCTAssertTrue(app.buttons["View 1 reply"].exists, "Nora's fold reads View 1 reply")
        XCTAssertFalse(element(app, "comment-row-aapl-logan").exists, "the answer is folded")
        screenshot(app, "logan-folded")
    }

    @MainActor
    func testTappingViewOneReplyShowsLogansAnswerOneIndentIn() {
        let app = launch("logan")
        let nora = element(app, "comment-body-aapl-nora")
        XCTAssertTrue(nora.waitForExistence(timeout: 15), "Nora's reply is drawn")

        element(app, "comment-fold-aapl-nora").tap()

        let logan = element(app, "comment-body-aapl-logan")
        XCTAssertTrue(logan.waitForExistence(timeout: 5), "Logan's answer shows")
        XCTAssertGreaterThan(logan.frame.minX, nora.frame.minX, "the answer sits one indent in")
        XCTAssertTrue(app.buttons["Hide replies"].exists, "the fold now offers Hide replies")
        screenshot(app, "logan-expanded")
    }

    @MainActor
    func testTappingChristellesQuoteShowsLogansAnswerAndHighlightsIt() {
        let app = launch("logan")
        let quote = element(app, "comment-quote-aapl-christelle")
        XCTAssertTrue(quote.waitForExistence(timeout: 15), "Christelle's quote is drawn")
        XCTAssertFalse(element(app, "comment-row-aapl-logan").exists, "the answer starts folded")

        quote.tap()

        let logan = element(app, "comment-row-aapl-logan")
        XCTAssertTrue(logan.waitForExistence(timeout: 5), "the jump expands Nora's fold")
        XCTAssertTrue(logan.isHittable, "the jump scrolls Logan's answer into view")
        screenshot(app, "logan-jumped")
    }

    @MainActor
    func testTappingAndresChipTogglesYourHeartAndShowsThreeOutlined() {
        let app = launch("logan")
        let chip = element(app, "reaction-chip-comment-aapl-andres-2764-FE0F")
        XCTAssertTrue(chip.waitForExistence(timeout: 15), "Andres's ❤️ chip is drawn")
        XCTAssertTrue(chip.label.contains("2 reactions"), "the chip starts at 2")
        XCTAssertFalse(chip.label.contains("yours"), "the heart is not yet yours")

        chip.tap()

        let after = NSPredicate(format: "label CONTAINS '3 reactions' AND label CONTAINS 'yours'")
        expectation(for: after, evaluatedWith: chip)
        waitForExpectations(timeout: 10)
        screenshot(app, "logan-reacted")
    }

    @MainActor
    func testTheRepliesDrawCompactChipsAndAReplyButtonButNoAddChip() {
        let app = launch("logan")
        XCTAssertTrue(element(app, "comment-row-aapl-christelle").waitForExistence(timeout: 15), "Christelle's row")
        XCTAssertTrue(element(app, "reaction-chip-comment-aapl-christelle-1F602").exists, "the 😂 chip")
        XCTAssertTrue(element(app, "reaction-chip-comment-aapl-christelle-2764-FE0F").exists, "the ❤️ chip")
        XCTAssertTrue(element(app, "comment-reply-aapl-christelle").exists, "Reply sits in the row")
        XCTAssertFalse(element(app, "reaction-add-comment-aapl-christelle").exists, "no + chip on a reply")
        let reply = element(app, "comment-reply-aapl-christelle")
        XCTAssertGreaterThanOrEqual(reply.frame.height, 44, "Reply is 44 tall")
        XCTAssertGreaterThanOrEqual(reply.frame.width, 44, "Reply is 44 wide")
    }

    @MainActor
    func testAPassedProposalShowsMonacosReplyFirstWithNoReplyButton() {
        let app = launch("passed")
        let monaco = element(app, "comment-row-nvda-monaco")
        XCTAssertTrue(monaco.waitForExistence(timeout: 15), "Monaco's reply is drawn")
        XCTAssertTrue(app.staticTexts["Monaco"].exists, "the name reads Monaco")
        XCTAssertTrue(
            app.staticTexts["Passed and filled. 38 → 39.82 sh NVDA in the treasury."].exists, "the outcome text")
        XCTAssertFalse(element(app, "comment-reply-nvda-monaco").exists, "Monaco's reply takes no Reply")
        let reply = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'comment-row-'")
        ).firstMatch
        XCTAssertEqual(reply.identifier, "comment-row-nvda-monaco", "Monaco's row is first")
        screenshot(app, "passed")
    }

    @MainActor
    func testAFailedProposalShowsMonacosReplyThenLogansWithItsChip() {
        let app = launch("failed")
        XCTAssertTrue(element(app, "comment-row-tsla-monaco").waitForExistence(timeout: 15), "Monaco's reply")
        XCTAssertFalse(element(app, "comment-reply-tsla-monaco").exists, "no Reply on Monaco's row")
        XCTAssertTrue(element(app, "reaction-chip-comment-tsla-logan-2764-FE0F").exists, "Logan's ❤️ chip")
        screenshot(app, "failed")
    }
}
