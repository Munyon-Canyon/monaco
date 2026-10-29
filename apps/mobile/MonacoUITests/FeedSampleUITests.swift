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
    func testEveryPostEndsItsRowWithTheReplyControl() {
        let app = launch("-feedHarness", "items")
        for (post, label) in Self.controls {
            let control = replyControl(app, post)
            XCTAssertTrue(control.exists, "no reply control on \(post)")
            XCTAssertEqual(control.label, label, "the control on \(post) reads wrong")
        }
    }

    @MainActor
    func testTheControlOnAMembersSaleOpensTheReplySheetOverTheFeed() {
        let app = launch("-feedHarness", "items")
        replyControl(app, Self.aaplSale).tap()
        XCTAssertTrue(app.otherElements["reply-sheet"].waitForExistence(timeout: 10), "no reply sheet")
        XCTAssertTrue(app.otherElements["reply-sheet-quote"].exists, "the sheet does not quote the sale")
        XCTAssertTrue(app.buttons["feed-filter"].exists, "the Feed is not behind the sheet")
    }

    @MainActor
    func testTheControlOnAProposalOpensItWithTheSheetUpAndNoQuote() {
        let app = launch("-feedHarness", "items")
        replyControl(app, Self.nvdaProposal).tap()
        XCTAssertTrue(app.otherElements["reply-sheet"].waitForExistence(timeout: 15), "no reply sheet")
        XCTAssertFalse(app.otherElements["reply-sheet-quote"].exists, "the sheet quotes something")
        XCTAssertTrue(app.buttons["proposal-share"].exists, "the proposal is not behind the sheet")
    }

    @MainActor
    func testTheControlOnACabalsResultOpensItsProposalAtTheReplies() {
        let app = launch("-feedHarness", "items")
        replyControl(app, Self.metaResult).tap()
        XCTAssertTrue(app.otherElements["reply-sheet"].waitForExistence(timeout: 15), "no reply sheet")
        XCTAssertFalse(app.otherElements["reply-sheet-quote"].exists, "the sheet quotes something")
        XCTAssertTrue(app.buttons["proposal-share"].exists, "the proposal is not behind the sheet")
    }

    @MainActor
    func testTheControlOnAReplyPostOpensTheProposalWithTheSheetQuotingIt() {
        let app = launch("-feedHarness", "items")
        replyControl(app, Self.tslaQuote).tap()
        XCTAssertTrue(app.otherElements["reply-sheet"].waitForExistence(timeout: 15), "no reply sheet")
        XCTAssertTrue(app.otherElements["reply-sheet-quote"].exists, "the sheet does not quote the reply")
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

    private static let nvdaProposal = "00000000-0000-7000-8000-0000000fe101"
    private static let metaResult = "00000000-0000-7000-8000-0000000fe008"
    private static let aaplSale = "00000000-0000-7000-8000-0000000fe009"
    private static let tslaQuote = "00000000-0000-7000-8000-0000000fe104"
    private static let openAIBuy = "00000000-0000-7000-8000-0000000fe010"

    private static let controls = [
        (nvdaProposal, "Reply, 6 replies"), (metaResult, "Reply, 5 replies"), (aaplSale, "Reply, 5 replies"),
        (tslaQuote, "Reply"), (openAIBuy, "Reply"),
    ]

    @MainActor
    private func replyControl(_ app: XCUIApplication, _ item: String) -> XCUIElement {
        let control = app.buttons["feed-reply-\(item)"]
        var swipes = 0
        while !control.waitForExistence(timeout: 3), swipes < 10 {
            app.swipeUp()
            swipes += 1
        }
        return control
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
