import XCTest

nonisolated final class PollSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func option(_ app: XCUIApplication, _ position: Int) -> XCUIElement {
        app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'poll-option-poll-' AND identifier ENDSWITH '-\(position)'")
        ).firstMatch
    }

    @MainActor
    func testCreateAThreeOptionPollVoteSeeTheBarsAndChangeTheVote() {
        let app = XCUIApplication()
        app.launchArguments = ["-composerHarness", "poll"]
        app.launch()
        let plus = app.buttons["comment-composer-plus"]
        XCTAssertTrue(plus.waitForExistence(timeout: 15), "no plus")
        plus.tap()
        element(app, "composer-menu-poll").tap()
        XCTAssertTrue(element(app, "poll-sheet").waitForExistence(timeout: 10), "no poll sheet")
        XCTAssertFalse(app.buttons["poll-post"].isEnabled, "Post is live on an empty poll")

        let question = app.textFields["poll-question-field"]
        question.tap()
        question.typeText("Trim before earnings?")
        for (index, label) in ["Yes", "No"].enumerated() {
            let field = app.textFields["poll-option-field-\(index + 1)"]
            field.tap()
            field.typeText(label)
        }
        app.buttons["poll-add-option"].tap()
        let third = app.textFields["poll-option-field-3"]
        third.tap()
        third.typeText("Wait")
        app.buttons["poll-post"].tap()

        XCTAssertTrue(option(app, 2).waitForExistence(timeout: 15), "no third option in the bubble")
        option(app, 0).tap()
        XCTAssertTrue(
            element(app, "poll-footer-poll-0").label.hasPrefix("1 vote"), "the vote did not count")
        XCTAssertTrue(option(app, 0).label.contains("100 percent"), "no bar share for the choice")
        XCTAssertEqual(option(app, 0).value as? String, "Your vote")
        option(app, 1).tap()
        XCTAssertEqual(option(app, 1).value as? String, "Your vote")
        XCTAssertEqual(option(app, 0).value as? String, "", "the old choice kept its check")
    }
}
