import XCTest

nonisolated final class ChatClosedSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launchChat(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoChatSampleQA", scenario]
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
    private func closedNotice(_ app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: "chat-closed").firstMatch
    }

    @MainActor
    private func composer(_ app: XCUIApplication) -> XCUIElement {
        app.textFields["chat-composer"]
    }

    @MainActor
    func testAThreadThatOpensClosedReplacesTheComposerAndStillOffersAWayBackIn() throws {
        let app = launchChat("-MonacoChatSampleClosedFirstLoad")

        let retry = app.buttons["chat-retry"]
        if !retry.waitForExistence(timeout: 40) {
            XCTFail(
                "a closed thread must leave the member something to tap. On screen:\n\(app.debugDescription.suffix(9000))"
            )
            return
        }
        XCTAssertTrue(
            app.descendants(matching: .any).matching(identifier: "chat-error").firstMatch.exists,
            "the screen should say why, next to the button"
        )
        XCTAssertTrue(closedNotice(app).exists, "the composer should be replaced by the closed notice")
        XCTAssertFalse(composer(app).exists, "a member who is out of the cabal cannot type into its chat")
        attachScreenshot(app, name: "01-opened-closed")

        retry.tap()

        XCTAssertTrue(
            composer(app).waitForExistence(timeout: 30),
            "asking again should reopen the thread, not stay shut on one refused load"
        )
        XCTAssertFalse(closedNotice(app).exists, "nothing should still be saying the thread is shut")
        attachScreenshot(app, name: "02-reopened")
    }
}
