import XCTest

nonisolated final class ChatSendSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launchChat(_ scenario: String...) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoChatSampleQA"] + scenario
        app.launch()
        return app
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func send(_ text: String, in app: XCUIApplication) {
        let composer = app.textFields["chat-composer"]
        XCTAssertTrue(composer.waitForExistence(timeout: 40), "the sample thread should load")
        composer.tap()
        composer.typeText(text)
        app.buttons["chat-send"].tap()
    }

    @MainActor
    func testAFailedSendShowsNotSentRetryAndTheRetryPostsOnce() throws {
        let app = launchChat("-MonacoChatSampleFlaky")

        send("offline", in: app)

        let retry = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH %@", "chat-retry-")).firstMatch
        XCTAssertTrue(retry.waitForExistence(timeout: 20), "a failed send should offer Not sent · Retry")
        XCTAssertEqual(retry.label, "Not sent · Retry")
        attachScreenshot(app, name: "01-not-sent")

        retry.tap()

        XCTAssertTrue(element(app, "chat-message-sent-9").waitForExistence(timeout: 20), "the retry should land")
        XCTAssertFalse(retry.exists, "a landed message has nothing left to retry")
        let copies = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH %@ AND label CONTAINS %@", "chat-message-", "offline"))
        XCTAssertEqual(copies.count, 1, "the message should appear once")
        attachScreenshot(app, name: "02-retried")
    }

    @MainActor
    func testASendAMemberWhoLeftMakesClosesTheChatAndKeepsTheThread() throws {
        let app = launchChat("-MonacoChatSampleClosedOnSend")

        send("still here?", in: app)

        XCTAssertTrue(element(app, "chat-closed").waitForExistence(timeout: 20), "the composer should be replaced")
        XCTAssertFalse(app.textFields["chat-composer"].exists)
        XCTAssertTrue(element(app, "chat-message-s8").exists, "the messages already loaded stay")
        attachScreenshot(app, name: "01-closed-on-send")
    }

    @MainActor
    func testTappingAnotherMembersNameOpensTheirProfile() throws {
        let app = launchChat()
        XCTAssertTrue(app.textFields["chat-composer"].waitForExistence(timeout: 40), "the sample thread should load")

        let author = app.buttons["chat-author-s1"]
        XCTAssertTrue(author.waitForExistence(timeout: 10), "the first message of a run names its author")
        author.tap()

        let opened = element(app, "chat-sample-opened-profile")
        XCTAssertTrue(opened.waitForExistence(timeout: 10))
        XCTAssertTrue(opened.label.contains("u-ana"), "it should open the author's profile: \(opened.label)")
        XCTAssertFalse(app.buttons["chat-author-s7"].exists, "the viewer's own messages show no author")
    }

    @MainActor
    func testReturningFromTheBackgroundDoesNotDuplicateRows() throws {
        let app = launchChat()
        XCTAssertTrue(element(app, "chat-message-s8").waitForExistence(timeout: 40), "the sample thread should load")
        let before = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH %@", "chat-message-")
        ).count

        XCUIDevice.shared.press(.home)
        app.activate()

        XCTAssertTrue(element(app, "chat-message-s8").waitForExistence(timeout: 20))
        let after = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH %@", "chat-message-")
        ).count
        XCTAssertEqual(after, before, "reopening must not add rows")
        attachScreenshot(app, name: "01-after-background")
    }
}
