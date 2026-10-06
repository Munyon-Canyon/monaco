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

        XCTAssertTrue(app.staticTexts["Nothing here yet."].waitForExistence(timeout: 15), "Everyone is plain empty")
        app.buttons["Following"].tap()
        XCTAssertTrue(
            app.staticTexts["Follow people to see what they do."].waitForExistence(timeout: 10),
            "Following asks a viewer who follows nobody to follow people")
        screenshot(app, "feed-follows-nobody")

        app.buttons["feed-find-friends"].tap()
        let friends = app.navigationBars["Friends on Monaco"]
        XCTAssertTrue(friends.waitForExistence(timeout: 10), "Find friends opens Friends")
        screenshot(app, "feed-find-friends")
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
