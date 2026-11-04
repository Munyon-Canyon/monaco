import XCTest

nonisolated final class ChatComposerMenuSampleUITests: XCTestCase {
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
    private func dismissSheet(_ sheet: XCUIElement, in app: XCUIApplication) {
        sheet.swipeDown()
        XCTAssertTrue(app.textFields["chat-composer"].waitForExistence(timeout: 10), "the composer should return")
    }

    @MainActor
    func testThePlusMenuListsGifAndStockAndOpensTheirSheets() throws {
        let app = launchChat("-MonacoChatSampleProposal")

        let composer = app.textFields["chat-composer"]
        XCTAssertTrue(composer.waitForExistence(timeout: 40), "the sample thread should load")
        composer.tap()
        composer.typeText("Anyone watching NVDA?")
        attachScreenshot(app, name: "01-can-send")

        element(app, "chat-composer-plus").tap()
        XCTAssertTrue(app.buttons["chat-menu-gif"].waitForExistence(timeout: 10), "the menu should list GIF")
        XCTAssertTrue(app.buttons["chat-menu-stock"].exists, "the menu should list Stock")
        XCTAssertFalse(app.buttons["chat-menu-thesis"].exists, "the sample has no thesis to share")
        attachScreenshot(app, name: "02-plus-menu")

        app.buttons["chat-menu-gif"].tap()
        let gifSheet = element(app, "gif-sheet")
        XCTAssertTrue(gifSheet.waitForExistence(timeout: 10), "the GIF sheet should open")
        attachScreenshot(app, name: "03-gif-sheet")
        dismissSheet(gifSheet, in: app)

        element(app, "chat-gif-chip").tap()
        let gifSheetAgain = element(app, "gif-sheet")
        XCTAssertTrue(gifSheetAgain.waitForExistence(timeout: 10), "the GIF chip should open the GIF sheet")
        dismissSheet(gifSheetAgain, in: app)

        element(app, "chat-composer-plus").tap()
        XCTAssertTrue(app.buttons["chat-menu-stock"].waitForExistence(timeout: 10))
        app.buttons["chat-menu-stock"].tap()
        XCTAssertTrue(element(app, "stock-sheet").waitForExistence(timeout: 10), "the stock sheet should open")
        attachScreenshot(app, name: "04-stock-sheet")
    }
}
