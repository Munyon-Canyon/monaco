import XCTest

nonisolated final class GifSheetSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String, host: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-gifHarness", scenario, "-gifHost", host]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func searchAndPick(_ app: XCUIApplication) {
        let attribution = element(app, "gif-attribution")
        XCTAssertTrue(attribution.waitForExistence(timeout: 15), "no attribution in trending")
        XCTAssertEqual(attribution.label, "Powered by GIPHY")
        XCTAssertTrue(app.staticTexts["Trending GIFs"].exists, "no trending title")
        XCTAssertTrue(element(app, "gif-cell-sample-gif-1").waitForExistence(timeout: 10), "no GIFs")
        let field = app.textFields["monaco-search-field"]
        field.tap()
        field.typeText("dance")
        XCTAssertTrue(app.staticTexts["GIFs"].waitForExistence(timeout: 10), "the title did not change")
        XCTAssertTrue(element(app, "gif-attribution").exists, "no attribution in search")
        let cell = element(app, "gif-cell-sample-gif-3")
        XCTAssertTrue(cell.waitForExistence(timeout: 10), "no search results")
        cell.tap()
    }

    @MainActor
    private func assertOneGifPost(_ app: XCUIApplication) {
        XCTAssertTrue(element(app, "gif-sheet").waitForNonExistence(timeout: 10), "the sheet stayed open")
        let posts = element(app, "gif-harness-posts")
        let sent = NSPredicate(format: "label == %@", "posts=1 kind=gif id=sample-gif-3")
        expectation(for: sent, evaluatedWith: posts)
        waitForExpectations(timeout: 15)
    }

    @MainActor
    func testTheDockedComposerOpensTheSheetAndSendsTheGif() {
        let app = launch("trending", host: "docked")
        let button = app.buttons["comment-composer-gif"]
        XCTAssertTrue(button.waitForExistence(timeout: 15), "no GIF button")
        button.tap()
        searchAndPick(app)
        assertOneGifPost(app)
    }

    @MainActor
    func testTheReplySheetOpensTheSheetAndSendsTheGif() {
        let app = launch("trending", host: "reply")
        XCTAssertTrue(element(app, "reply-sheet-quote").waitForExistence(timeout: 15), "no reply sheet")
        app.buttons["comment-composer-gif"].tap()
        searchAndPick(app)
        assertOneGifPost(app)
    }

    @MainActor
    func testAFailingProviderShowsTheCopyAndRetry() {
        let app = launch("failed", host: "sheet")
        XCTAssertTrue(app.staticTexts["GIFs aren’t available right now."].waitForExistence(timeout: 15), "no copy")
        XCTAssertTrue(app.buttons["gif-retry"].exists, "no Retry")
    }

    @MainActor
    func testNoMatchesNamesTheQuery() {
        let app = launch("empty", host: "sheet")
        XCTAssertTrue(app.staticTexts["No GIFs for “zzzz”"].waitForExistence(timeout: 15), "no empty copy")
        XCTAssertTrue(element(app, "gif-attribution").exists, "no attribution")
    }
}
