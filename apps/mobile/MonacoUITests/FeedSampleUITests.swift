import XCTest

nonisolated final class FeedSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testFollowingNobodyAsksToFollowAndFindFriendsOpensFriends() {
        let app = XCUIApplication()
        app.launchArguments = ["-feedHarness", "followsNobody"]
        app.launch()

        XCTAssertTrue(app.staticTexts["No updates yet"].waitForExistence(timeout: 15), "Everyone is plain empty")
        app.buttons["feed-filter"].tap()
        app.buttons["feed-scope-Following"].tap()
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.95)).tap()
        XCTAssertTrue(
            app.staticTexts["Follow people to see what they do."].waitForExistence(timeout: 10),
            "Following asks a viewer who follows nobody to follow people")
        screenshot(app, "feed-follows-nobody")

        app.buttons["feed-find-friends"].tap()
        let friends = app.navigationBars["Find friends"]
        XCTAssertTrue(friends.waitForExistence(timeout: 10), "Find friends opens Friends")
        screenshot(app, "feed-find-friends")
    }

    @MainActor
    func testFilterMenuAppliesEachPickAtOnceAndStaysOpen() {
        let app = launch("-feedHarness", "items")
        let filter = app.buttons["feed-filter"]
        XCTAssertTrue(filter.waitForExistence(timeout: 15), "the filter round is missing")
        filter.tap()
        let trades = app.buttons["feed-chip-Trades"]
        XCTAssertTrue(trades.waitForExistence(timeout: 5), "the menu did not open: \(app.debugDescription)")
        trades.tap()
        XCTAssertTrue(app.buttons["feed-chip-Price moves"].exists, "the menu closed after one pick")
        XCTAssertTrue(app.buttons["feed-muted-link"].exists, "the menu has no Muted item")
        app.buttons["feed-scope-Following"].tap()
        XCTAssertTrue(app.buttons["feed-chip-Cabals"].exists, "the menu closed after the second pick")
        app.buttons["feed-muted-link"].tap()
        XCTAssertTrue(app.navigationBars["Muted"].waitForExistence(timeout: 5), "Muted did not open")
    }

    @MainActor
    func testLikeTogglesLocallyAndShareExplainsItself() {
        let app = launch("-feedHarness", "items")
        let like = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'feed-like-'")).firstMatch
        XCTAssertTrue(like.waitForExistence(timeout: 15), "no like button")
        XCTAssertFalse(like.isSelected)
        like.tap()
        XCTAssertTrue(like.isSelected, "the like did not stick")
        let share = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'feed-share-'")).firstMatch
        share.tap()
        XCTAssertTrue(app.staticTexts["Sharing opens soon."].waitForExistence(timeout: 5), "no share toast")
    }

    @MainActor
    func testALoadMoreFailureShowsTheNoticeAndRetryLoadsTheNextPage() {
        let app = launch("-feedHarness", "loadMoreFailed")
        let retry = app.buttons["feed-load-more-retry"]
        var swipes = 0
        while !retry.exists, swipes < 8 {
            app.swipeUp()
            swipes += 1
        }
        XCTAssertTrue(retry.waitForExistence(timeout: 10), "no Retry older updates")
        XCTAssertTrue(
            app.staticTexts["Older updates couldn’t load. The items above are still here."].exists, "no notice")
        retry.tap()
        XCTAssertTrue(
            app.staticTexts["feed-caught-up"].waitForExistence(timeout: 10)
                || app.staticTexts["You’re caught up"].waitForExistence(timeout: 10),
            "the retry did not load the last page")
    }

    @MainActor
    func testReplyingShowsItsTargetAndAnEmptyFieldCannotSend() {
        let app = launch("-feedPostHarness", "ready")
        let send = app.buttons["comment-composer-send"]
        XCTAssertTrue(send.waitForExistence(timeout: 15), "no send disc")
        XCTAssertFalse(send.isEnabled, "an empty field can send")
        let reply = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'comment-reply-'")).firstMatch
        XCTAssertTrue(reply.waitForExistence(timeout: 10), "no Reply")
        reply.tap()
        XCTAssertTrue(app.staticTexts["Replying to Mia"].waitForExistence(timeout: 5), "no reply target")
        app.buttons["comment-composer-cancel-reply"].tap()
        XCTAssertTrue(app.staticTexts["Replying to Mia"].waitForNonExistence(timeout: 5), "cancel kept the target")
    }

    @MainActor
    func testAFailedSendKeepsTheTextAndShowsTheNotice() {
        let app = launch("-feedPostHarness", "failed")
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

    @MainActor
    func testAnUnavailablePostHidesTheComposerAndGoesBackToTheFeed() {
        let app = launch("-feedPostHarness", "unavailable")
        let back = app.buttons["Back to feed"]
        XCTAssertTrue(back.waitForExistence(timeout: 15), "no Back to feed")
        XCTAssertFalse(app.buttons["comment-composer-send"].exists, "the composer is still there")
        back.tap()
        XCTAssertTrue(app.textFields["monaco-search-field"].waitForExistence(timeout: 10), "did not return to the feed")
    }

    @MainActor
    private func launch(_ arguments: String...) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = arguments
        app.launch()
        return app
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
