import XCTest

nonisolated final class ComposerMenuSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-composerHarness", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testThePlusShowsOnADockedComposerForTheStockRow() {
        let app = launch("docked")
        XCTAssertTrue(app.textFields["comment-composer-field"].waitForExistence(timeout: 15), "no composer")
        XCTAssertTrue(app.buttons["comment-composer-plus"].exists, "no plus")
    }

    @MainActor
    func testThePlusOpensTheMenuWithTheHarnessRows() {
        let app = launch("menu")
        let plus = app.buttons["comment-composer-plus"]
        XCTAssertTrue(plus.waitForExistence(timeout: 15), "no plus")
        plus.tap()
        XCTAssertTrue(element(app, "composer-menu-poll").waitForExistence(timeout: 10), "no Poll row")
        XCTAssertTrue(element(app, "composer-menu-thesis").exists, "no thesis row")
        XCTAssertTrue(element(app, "composer-menu-stock").exists, "no stock row")
    }

    @MainActor
    func testTheThesisRowIsAbsentOnASale() {
        let app = launch("menuSale")
        let plus = app.buttons["comment-composer-plus"]
        XCTAssertTrue(plus.waitForExistence(timeout: 15), "no plus")
        plus.tap()
        XCTAssertTrue(element(app, "composer-menu-poll").waitForExistence(timeout: 10), "no Poll row")
        XCTAssertTrue(element(app, "composer-menu-stock").exists, "no stock row on a sale")
        XCTAssertFalse(element(app, "composer-menu-thesis").exists, "the thesis row shows on a sale")
    }

    @MainActor
    func testQuotingTheThesisShowsThePreview() {
        let app = launch("menu")
        let plus = app.buttons["comment-composer-plus"]
        XCTAssertTrue(plus.waitForExistence(timeout: 15), "no plus")
        plus.tap()
        element(app, "composer-menu-thesis").tap()
        XCTAssertTrue(app.buttons["thesis-quote-all"].waitForExistence(timeout: 10), "no sheet")
        app.buttons["thesis-quote-all"].tap()
        XCTAssertTrue(element(app, "composer-attachment-preview").waitForExistence(timeout: 10), "no preview")
    }

    @MainActor
    func testAThesisQuoteReplyRendersItsBubble() {
        let app = launch("thesisBubble")
        XCTAssertTrue(element(app, "attachment-thesis-c1").waitForExistence(timeout: 15), "no thesis bubble")
    }

    @MainActor
    func testAStockReplyRendersItsBubble() {
        let app = launch("stockBubble")
        XCTAssertTrue(element(app, "attachment-stock-c2").waitForExistence(timeout: 15), "no stock bubble")
    }

    @MainActor
    func testPickingAddsThePreviewAndTheCrossClearsIt() {
        let app = launch("menu")
        let plus = app.buttons["comment-composer-plus"]
        XCTAssertTrue(plus.waitForExistence(timeout: 15), "no plus")
        plus.tap()
        element(app, "composer-menu-stock").tap()
        let row = element(app, "stock-share-row-AAPL")
        XCTAssertTrue(row.waitForExistence(timeout: 10), "no stock row in the sheet")
        row.tap()
        XCTAssertTrue(element(app, "composer-attachment-preview").waitForExistence(timeout: 10), "no preview")
        app.buttons["composer-attachment-remove"].tap()
        XCTAssertFalse(element(app, "composer-attachment-preview").exists, "the draft was not cleared")
    }

    @MainActor
    func testTheDraftHarnessShowsTheSamplePreviewAndTheCrossClearsIt() {
        let app = launch("draft")
        XCTAssertTrue(element(app, "composer-attachment-preview").waitForExistence(timeout: 15), "no preview")
        app.buttons["composer-attachment-remove"].tap()
        XCTAssertFalse(element(app, "composer-attachment-preview").exists, "the draft was not cleared")
    }

    @MainActor
    func testAGifReplyStillRendersThroughTheAttachmentBubble() {
        let app = XCUIApplication()
        app.launchArguments = ["-gifHarness", "bubble"]
        app.launch()
        let bubble = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'gif-bubble-'")
        ).firstMatch
        XCTAssertTrue(bubble.waitForExistence(timeout: 15), "no GIF bubble")
    }
}
