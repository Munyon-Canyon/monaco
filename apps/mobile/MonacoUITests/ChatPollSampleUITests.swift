import XCTest

nonisolated final class ChatPollSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launchChat() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoChatSampleQA", "-MonacoChatSamplePoll"]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func option(_ app: XCUIApplication, _ messageID: String, _ position: Int) -> XCUIElement {
        element(app, "poll-option-\(messageID)-\(position)")
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testVoteMoveAndWithdrawInTheThread() {
        let app = launchChat()
        XCTAssertTrue(app.textFields["chat-composer"].waitForExistence(timeout: 40), "the sample thread should load")

        let first = option(app, "s6q", 0)
        XCTAssertTrue(first.waitForExistence(timeout: 15), "the poll bubble should be in the thread")
        attachScreenshot(app, name: "01-poll-in-thread")
        first.tap()
        XCTAssertEqual(first.value as? String, "Your vote")
        XCTAssertTrue(element(app, "poll-footer-s6q").label.hasPrefix("6 votes"), "the vote should count")
        attachScreenshot(app, name: "02-voted")

        option(app, "s6q", 1).tap()
        XCTAssertEqual(option(app, "s6q", 1).value as? String, "Your vote")
        XCTAssertEqual(first.value as? String, "", "the old choice should lose its check")
        XCTAssertTrue(element(app, "poll-footer-s6q").label.hasPrefix("6 votes"), "a moved vote keeps the total")

        option(app, "s6q", 1).tap()
        XCTAssertEqual(option(app, "s6q", 1).value as? String, "", "tapping the choice again withdraws it")
        XCTAssertTrue(element(app, "poll-footer-s6q").label.hasPrefix("5 votes"), "a withdrawn vote leaves the count")
    }

    @MainActor
    func testPostAPollFromThePlusMenu() {
        let app = launchChat()
        XCTAssertTrue(app.textFields["chat-composer"].waitForExistence(timeout: 40), "the sample thread should load")

        element(app, "chat-composer-plus").tap()
        XCTAssertTrue(app.buttons["chat-menu-poll"].waitForExistence(timeout: 10), "the menu should list Poll")
        attachScreenshot(app, name: "03-plus-menu")
        app.buttons["chat-menu-poll"].tap()
        XCTAssertTrue(element(app, "poll-sheet").waitForExistence(timeout: 10), "the poll sheet should open")

        let question = app.textFields["poll-question-field"]
        question.tap()
        question.typeText("Pizza tonight?")
        for (index, label) in ["Yes", "No"].enumerated() {
            let field = app.textFields["poll-option-field-\(index + 1)"]
            field.tap()
            field.typeText(label)
        }
        app.buttons["poll-post"].tap()

        let posted = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'poll-option-sent-' AND identifier ENDSWITH '-0'")
        ).firstMatch
        XCTAssertTrue(posted.waitForExistence(timeout: 15), "the posted poll should appear as a bubble")
        attachScreenshot(app, name: "04-posted")
        posted.tap()
        XCTAssertEqual(posted.value as? String, "Your vote")
    }
}
