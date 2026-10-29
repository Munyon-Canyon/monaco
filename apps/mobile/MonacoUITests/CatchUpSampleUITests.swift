import XCTest

nonisolated final class CatchUpSampleUITests: XCTestCase {
    private static let tsla = "proposal:00000000-0000-7000-8000-00000000b005"

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testSwipingRightShowsClearAndClearingRemovesTheRow() {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCatchUpSample", "six"]
        app.launch()
        let row = app.descendants(matching: .any).matching(identifier: "catch-up-row-\(Self.tsla)").firstMatch
        for _ in 0..<4 where !row.waitForExistence(timeout: 3) { app.swipeUp() }
        XCTAssertTrue(row.exists, "the six sample did not open")

        let start = row.coordinate(withNormalizedOffset: CGVector(dx: 0.1, dy: 0.5))
        let end = row.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.5))
        start.press(forDuration: 0.1, thenDragTo: end, withVelocity: .slow, thenHoldForDuration: 1)
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = "catch-up-swipe-clear"
        attachment.lifetime = .keepAlways
        add(attachment)

        let clear = app.buttons["Clear"].firstMatch
        XCTAssertTrue(clear.waitForExistence(timeout: 5), "the swipe shows Clear")
        clear.tap()
        let cleared = app.descendants(matching: .any).matching(identifier: "catch-up-section-cleared").firstMatch
        XCTAssertTrue(cleared.waitForExistence(timeout: 5), "the cleared row moves to Cleared today")
    }

    @MainActor
    func testCaughtUpShowsTheClearedRowsAndOpenFeed() {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCatchUpSample", "caughtUp"]
        app.launch()
        XCTAssertTrue(app.staticTexts["You're caught up"].waitForExistence(timeout: 10), "the caught-up state shows")
        XCTAssertTrue(app.buttons["catch-up-open-feed"].exists, "Open Feed is offered")
        XCTAssertTrue(
            app.descendants(matching: .any).matching(identifier: "catch-up-section-cleared").firstMatch.exists,
            "Cleared today is listed")
    }

    @MainActor
    private func openDeck(_ scenario: String = "deck") -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCatchUpSample", scenario]
        app.launch()
        XCTAssertTrue(
            app.staticTexts["catch-up-deck-position"].waitForExistence(timeout: 10), "the deck did not open")
        return app
    }

    @MainActor
    func testTheDeckOpensOnTheMentionWithItsCommentChipAndDock() {
        let app = openDeck()
        XCTAssertEqual(app.staticTexts["catch-up-deck-position"].label, "2 of 6")
        XCTAssertTrue(app.buttons["catch-up-deck-keep"].exists)
        XCTAssertTrue(app.buttons["catch-up-deck-clear"].exists)
        XCTAssertTrue(app.buttons["catch-up-deck-reply"].exists)
        XCTAssertTrue(app.buttons["catch-up-deck-open"].exists)
        XCTAssertTrue(
            app.descendants(matching: .any).matching(identifier: "catch-up-deck-see-all").firstMatch.exists)
        let chip = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'red heart'")).firstMatch
        XCTAssertTrue(chip.exists, "the heart chip shows")
        XCTAssertGreaterThanOrEqual(chip.frame.height, 44)
        XCTAssertGreaterThanOrEqual(chip.frame.width, 44)
        let reply = app.buttons["catch-up-deck-comment-reply"]
        XCTAssertGreaterThanOrEqual(reply.frame.height, 44)
        XCTAssertGreaterThanOrEqual(reply.frame.width, 44)
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = "catch-up-deck"
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testClearAdvancesAndKeepAdvances() {
        let app = openDeck()
        app.buttons["catch-up-deck-clear"].tap()
        XCTAssertTrue(app.staticTexts["3 of 6"].waitForExistence(timeout: 5), "Clear shows item 3")
        app.buttons["catch-up-deck-keep"].tap()
        XCTAssertTrue(app.staticTexts["4 of 6"].waitForExistence(timeout: 5), "Keep shows item 4")
    }

    @MainActor
    func testSwipingTheCardRightClearsAndLeftKeeps() {
        let app = openDeck()
        let card = app.otherElements["catch-up-deck-comment"].firstMatch
        XCTAssertTrue(card.exists)
        let start = card.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.5))
        start.press(forDuration: 0.05, thenDragTo: card.coordinate(withNormalizedOffset: CGVector(dx: 2.4, dy: 0.5)))
        XCTAssertTrue(app.staticTexts["3 of 6"].waitForExistence(timeout: 5), "a right swipe clears")
        let window = app.windows.firstMatch
        let from = window.coordinate(withNormalizedOffset: CGVector(dx: 0.8, dy: 0.3))
        from.press(forDuration: 0.05, thenDragTo: window.coordinate(withNormalizedOffset: CGVector(dx: 0.05, dy: 0.3)))
        XCTAssertTrue(app.staticTexts["4 of 6"].waitForExistence(timeout: 5), "a left swipe keeps")
    }

    @MainActor
    func testWorkingToTheEndClosesTheDeckToTheList() {
        let app = openDeck("deckPostReply")
        for _ in 0..<3 {
            app.buttons["catch-up-deck-clear"].tap()
            _ = app.buttons["catch-up-deck-clear"].waitForExistence(timeout: 2)
        }
        XCTAssertTrue(app.staticTexts["You're caught up"].waitForExistence(timeout: 10), "the list shows caught up")
    }

    @MainActor
    func testAPostReplyDrawsThePostHeadAndOpenPostWithNoTally() {
        let app = openDeck("deckPostReply")
        XCTAssertTrue(app.staticTexts["Logan · sold AAPL"].exists)
        XCTAssertEqual(app.buttons["catch-up-deck-open"].label, "Open post")
        XCTAssertFalse(
            app.descendants(matching: .any).matching(identifier: "catch-up-deck-vote-ends").firstMatch.exists)
    }
}
