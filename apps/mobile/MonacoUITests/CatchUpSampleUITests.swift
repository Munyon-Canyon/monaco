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
        XCTAssertTrue(row.waitForExistence(timeout: 10), "the six sample did not open")

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
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: row)
        waitForExpectations(timeout: 5)
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
}
