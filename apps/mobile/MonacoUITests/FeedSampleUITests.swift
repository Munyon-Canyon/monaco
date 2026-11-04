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

        XCTAssertTrue(
            app.staticTexts["Quiet week"].waitForExistence(timeout: 15), "Your cabals and friends is a quiet week")
        app.buttons["feed-filter"].tap()
        FeedBrowseJourney.menuItem(app, "feed-scope-Friends").tap()
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
    func testFeedRootShowsTheTabBarWithFeedSelected() {
        let app = launch("-feedHarness", "items")
        XCTAssertTrue(app.buttons["feed-filter"].waitForExistence(timeout: 15), "the feed did not load")
        XCTAssertTrue(app.tabBars.firstMatch.exists, "the Feed root draws the tab bar")
        XCTAssertEqual(app.tabBars.buttons.count, 5, "the bar has the five roots")
        XCTAssertTrue(app.tabBars.buttons["Feed"].isSelected, "Feed is the selected tab")
    }

    @MainActor
    func testFilterMenuListsTheFourScopesAndMarksTheChosenOne() {
        let app = launch("-feedHarness", "items")
        let filter = app.buttons["feed-filter"]
        XCTAssertTrue(filter.waitForExistence(timeout: 15), "the filter round is missing")
        filter.tap()
        let network = FeedBrowseJourney.menuItem(app, "feed-scope-Your cabals and friends")
        XCTAssertTrue(network.waitForExistence(timeout: 5), "the menu did not open: \(app.debugDescription)")
        XCTAssertTrue(network.isSelected, "Your cabals and friends is not checked by default")
        for option in FeedBrowseJourney.filterOptions {
            XCTAssertTrue(FeedBrowseJourney.menuItem(app, option).exists, "no \(option) in the menu")
        }
        FeedBrowseJourney.menuItem(app, "feed-scope-Everyone").tap()
        filter.tap()
        XCTAssertTrue(
            FeedBrowseJourney.menuItem(app, "feed-scope-Everyone").waitForExistence(timeout: 5)
                && FeedBrowseJourney.menuItem(app, "feed-scope-Everyone").isSelected,
            "Everyone did not check")
        FeedBrowseJourney.menuItem(app, "feed-muted-link").tap()
        XCTAssertTrue(app.navigationBars["Muted"].waitForExistence(timeout: 5), "Muted did not open")
    }

    @MainActor
    func testAReactionSticksAndReplyOpensThePost() {
        let app = launch("-feedHarness", "items")
        let id = "00000000-0000-7000-8000-0000000fe105"
        let key = "feed-\(id)"
        XCTAssertTrue(app.buttons["feed-filter"].waitForExistence(timeout: 15), "the feed did not load")
        FeedReactAndReplyJourney.addReaction(app, to: key, search: "fire", hex: "1F525", step: "feed sample")
        let chip = app.element("reaction-chip-\(key)-1F525")
        XCTAssertTrue(chip.waitForExistence(timeout: 10), "the reaction did not stick as a chip")
        XCTAssertTrue(chip.label.contains("1"), "the chip does not count the reaction: \(chip.label)")

        let reply = app.element("feed-reply-\(id)")
        app.scrollIntoReach(reply)
        XCTAssertTrue(reply.waitForExistence(timeout: 5), "no reply control on the post")
        reply.tap()
        let close = app.element("reply-sheet-close")
        XCTAssertTrue(close.waitForExistence(timeout: 10), "the reply control opened no reply sheet")
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
        XCTAssertTrue(app.staticTexts["Reply to Mia"].waitForExistence(timeout: 5), "no reply sheet")
        app.buttons["reply-sheet-close"].tap()
        XCTAssertTrue(app.staticTexts["Reply to Mia"].waitForNonExistence(timeout: 5), "close kept the sheet")
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
        XCTAssertTrue(app.buttons["feed-filter"].waitForExistence(timeout: 10), "did not return to the feed")
    }

    @MainActor
    func testALongPressOnAPostsReactionRowOpensTheTrayNotThePostMenu() {
        let app = launch("-feedHarness", "items")
        XCTAssertTrue(app.buttons["feed-filter"].waitForExistence(timeout: 15), "the feed did not load")
        let row = app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH %@", "reaction-row-feed-")).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10), "no reaction row on a feed post")
        row.press(forDuration: 0.6)
        let tray = app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH %@", "reaction-tray-")).firstMatch
        XCTAssertTrue(
            tray.waitForExistence(timeout: 5),
            "the long press did not open the reaction tray: \(app.debugDescription)")
        XCTAssertFalse(app.buttons["feed-report"].exists, "the post menu opened over the tray")
        XCTAssertFalse(app.buttons["feed-block"].exists, "the post menu opened over the tray")
        screenshot(app, "feed-reaction-row-long-press")
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
