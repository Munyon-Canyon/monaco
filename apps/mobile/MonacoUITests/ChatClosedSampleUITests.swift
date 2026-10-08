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

        if !closedNotice(app).waitForExistence(timeout: 40) {
            XCTFail(
                "a thread that opens closed must say so. On screen:\n\(app.debugDescription.suffix(9000))"
            )
            return
        }
        XCTAssertFalse(composer(app).exists, "a member who is out of the cabal cannot type into its chat")
        XCTAssertFalse(app.buttons["chat-retry"].exists, "a closed thread is not a failed load to retry")
        XCTAssertFalse(
            app.descendants(matching: .any).matching(identifier: "chat-error").firstMatch.exists,
            "a closed thread must not also claim the load failed"
        )
        attachScreenshot(app, name: "01-opened-closed")

        XCUIDevice.shared.press(.home)
        app.activate()

        XCTAssertTrue(
            composer(app).waitForExistence(timeout: 30),
            "opening again should reopen the thread, not stay shut on one refused load"
        )
        XCTAssertFalse(closedNotice(app).exists, "nothing should still be saying the thread is shut")
        attachScreenshot(app, name: "02-reopened")
    }
}
