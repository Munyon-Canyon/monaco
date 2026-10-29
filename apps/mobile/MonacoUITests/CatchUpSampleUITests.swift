import XCTest

nonisolated final class CatchUpSampleUITests: XCTestCase {
    private static let tslaRow = "catch-up-row-proposal:00000000-0000-7000-8000-00000000b005"

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testSwipingARowRightRevealsClearAndClearingRemovesIt() {
        let app = launch("six")
        let row = app.descendants(matching: .any).matching(identifier: Self.tslaRow).firstMatch
        for _ in 0..<3 where !row.exists || !row.isHittable { app.swipeUp() }
        XCTAssertTrue(row.waitForExistence(timeout: 10), "the TSLA result row is on the list")

        let start = row.coordinate(withNormalizedOffset: CGVector(dx: 0.15, dy: 0.5))
        let end = row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
        start.press(forDuration: 0.05, thenDragTo: end)

        let clear = app.buttons["Clear"]
        XCTAssertTrue(clear.waitForExistence(timeout: 5), "swiping right shows the Clear action")
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = "catch-up-mid-swipe"
        shot.lifetime = .keepAlways
        add(shot)

        clear.tap()
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: row)
        waitForExpectations(timeout: 5)
        XCTAssertTrue(
            app.buttons["catch-up-row-proposal:00000000-0000-7000-8000-00000000b004"].exists, "the other result stays")
    }

    @MainActor
    func testCaughtUpShowsTheClearedRowsAndOpenFeed() {
        let app = launch("caughtUp")
        XCTAssertTrue(app.staticTexts["You're caught up"].waitForExistence(timeout: 10))
        XCTAssertTrue(
            app.staticTexts[
                "Nothing in AC Capital, Tech Titans or Degens waits on you. The next vote lands here first."
            ]
            .exists, "the line names the viewer's cabals")
        XCTAssertTrue(app.buttons["catch-up-open-feed"].exists)
        XCTAssertFalse(app.buttons["catch-up-one-by-one"].exists, "the capsule is hidden when caught up")
        XCTAssertTrue(app.staticTexts["AC Capital · you voted Yes · filled"].exists)
        XCTAssertTrue(app.staticTexts["AC Capital · you replied"].exists)
        XCTAssertTrue(app.staticTexts["5 members · later"].exists)
    }

    @MainActor
    func testFirstUseShowsTheNoCabalLine() {
        let app = launch("firstUse")
        XCTAssertTrue(app.staticTexts["You're caught up"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.staticTexts["When a cabal you're in votes, it lands here first."].exists)
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCatchUpSample", scenario]
        app.launch()
        XCTAssertTrue(
            app.descendants(matching: .any).matching(identifier: "catch-up-page").firstMatch
                .waitForExistence(timeout: 10),
            "the Catch up harness did not open"
        )
        return app
    }
}
