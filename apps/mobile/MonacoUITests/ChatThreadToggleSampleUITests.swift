import XCTest

nonisolated final class ChatThreadToggleSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func openThread() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoChatThreadSampleQA"]
        app.launch()
        let root = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH %@ AND label CONTAINS %@", "chat-message-", "Apple reports")
        ).firstMatch
        XCTAssertTrue(root.waitForExistence(timeout: 40), "the sample channel should load")
        root.press(forDuration: 1.2)
        app.buttons["Reply"].tap()
        XCTAssertTrue(app.textFields["chat-composer"].waitForExistence(timeout: 20), "the thread should open")
        return app
    }

    @MainActor
    private func reply(_ text: String, in app: XCUIApplication) {
        let composer = app.textFields["chat-composer"]
        composer.tap()
        composer.typeText(text)
        app.buttons["chat-send"].tap()
        let sent = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH %@ AND label CONTAINS %@", "chat-message-", text)
        ).firstMatch
        XCTAssertTrue(sent.waitForExistence(timeout: 20), "the reply should show in the thread")
    }

    @MainActor
    private func backToChannel(_ app: XCUIApplication) {
        app.navigationBars.buttons.firstMatch.tap()
        XCTAssertTrue(app.element("chat-title").waitForExistence(timeout: 20), "back on the channel")
    }

    @MainActor
    private func threadHeaders(_ app: XCUIApplication) -> XCUIElementQuery {
        app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH %@", "chat-thread-header-"))
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testAReplySentWithTheSwitchOnShowsUnderARepliedToAThreadHeaderInTheChannel() throws {
        let app = openThread()
        let toggle = app.switches["chat-thread-also-in-channel"]
        XCTAssertTrue(toggle.waitForExistence(timeout: 10))
        XCTAssertEqual(toggle.value as? String, "0", "the switch starts off")

        toggle.coordinate(withNormalizedOffset: CGVector(dx: 0.92, dy: 0.5)).tap()
        XCTAssertEqual(toggle.value as? String, "1", "tapping the switch turns it on")
        reply("agree with Ana", in: app)
        XCTAssertEqual(toggle.value as? String, "0", "the switch resets after a send")

        backToChannel(app)
        XCTAssertTrue(
            threadHeaders(app).firstMatch.waitForExistence(timeout: 20),
            "the reply should show in the channel under a replied to a thread header")
        attachScreenshot(app, name: "01-channel-header")
    }

    @MainActor
    func testAReplySentWithTheSwitchOffStaysOutOfTheChannel() throws {
        let app = openThread()
        reply("thread only", in: app)

        backToChannel(app)
        XCTAssertTrue(
            app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "1 reply")).firstMatch
                .waitForExistence(timeout: 20),
            "the channel row should count the reply")
        XCTAssertEqual(threadHeaders(app).count, 0, "a thread only reply has no channel row")
    }
}
