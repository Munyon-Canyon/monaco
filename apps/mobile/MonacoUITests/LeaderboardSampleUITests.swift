import XCTest

nonisolated final class LeaderboardSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testPeopleShowsTheLargeTitleAboveThePeriodLineAndNoDockedCard() {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoLeaderboardSample", "peopleMonth"]
        app.launch()
        let title = element(app, "leaderboard-large-title")
        let period = element(app, "leaderboard-period-line")
        XCTAssertTrue(period.waitForExistence(timeout: 10), "the period line shows")
        XCTAssertTrue(title.exists, "F's large Leaderboard title shows under the back and info buttons")
        XCTAssertLessThan(title.frame.minY, period.frame.minY, "the title sits above the period line")
        XCTAssertTrue(app.tabBars.buttons["Home"].isSelected, "the board is opened from Home")
        XCTAssertFalse(
            element(app, "leaderboard-docked-row").exists, "the You row is in the list, not in a floating card")
    }

    @MainActor
    func testFriendsShowsTopOfYourFriendsAndTheLastRowClearsTheTabBar() {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoLeaderboardSample", "friendsMonth"]
        app.launch()
        XCTAssertTrue(element(app, "leaderboard-period-line").waitForExistence(timeout: 10), "the board shows")
        XCTAssertTrue(app.staticTexts["Top of your friends"].exists, "the podium has F's heading")
        for _ in 0..<4 { app.swipeUp() }
        let rows = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'leaderboard-row-'"))
        let last = rows.allElementsBoundByIndex.max { $0.frame.maxY < $1.frame.maxY }
        XCTAssertNotNil(last, "rows show")
        XCTAssertLessThanOrEqual(
            last?.frame.maxY ?? .infinity, app.tabBars.firstMatch.frame.minY + 1,
            "the last row ends above the tab bar")
        XCTAssertFalse(
            element(app, "leaderboard-large-title").isHittable, "scrolled, the large title has folded into the bar")
    }

    @MainActor
    func testSampleScrollPodiumBringsTheLeadLineOnScreenAtAccessibilityXXXLarge() {
        let app = XCUIApplication()
        app.launchArguments = [
            "-MonacoLeaderboardSample", "friendsMonth",
            "-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL",
            "-MonacoSampleScroll", "podium",
        ]
        app.launch()
        let lead = element(app, "leaderboard.podium.lead")
        XCTAssertTrue(lead.waitForExistence(timeout: 10), "the right-calls-ahead line shows")
        XCTAssertTrue(lead.isHittable, "the podium section scrolled into view")
        XCTAssertTrue(
            app.windows.firstMatch.frame.contains(lead.frame), "the lead line sits inside the window at AX XXXL")
    }
}
