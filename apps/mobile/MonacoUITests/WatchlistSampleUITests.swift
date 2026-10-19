import XCTest

nonisolated final class WatchlistSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String, textSize: String? = nil) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoWatchlistSample", scenario]
        if let textSize {
            app.launchArguments += ["-UIPreferredContentSizeCategoryName", textSize]
        }
        app.launch()
        return app
    }

    @MainActor
    private func anyElement(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func waitForScreen(_ app: XCUIApplication, _ context: String) {
        XCTAssertTrue(
            app.navigationBars["Your watchlist"].waitForExistence(timeout: 30),
            "\(context) never drew Your watchlist"
        )
    }

    @MainActor
    func testEveryScenarioDraws() throws {
        for scenario in ["ready", "empty", "noResults", "loading", "failed"] {
            let app = launch(scenario)
            waitForScreen(app, scenario)
            attachScreenshot(app, name: "watchlist-\(scenario)")
            app.terminate()
        }
    }

    @MainActor
    func testStarsToggleAndTheCountFollows() throws {
        let app = launch("ready")
        waitForScreen(app, "ready")

        XCTAssertTrue(app.staticTexts["3 stocks selected"].waitForExistence(timeout: 25), "the count never drew")
        let tesla = app.buttons["watchlist-star-TSLAx"]
        XCTAssertTrue(tesla.waitForExistence(timeout: 25), "the Tesla star is missing")
        XCTAssertEqual(tesla.label, "Add Tesla")
        XCTAssertFalse(tesla.isSelected)
        XCTAssertTrue(app.buttons["watchlist-star-NVDAx"].isSelected, "a watched stock reads as selected")

        tesla.tap()
        XCTAssertTrue(app.staticTexts["4 stocks selected"].waitForExistence(timeout: 10), "the star did not count")
        XCTAssertEqual(tesla.label, "Remove Tesla")
        XCTAssertTrue(tesla.isSelected)

        tesla.tap()
        XCTAssertTrue(app.staticTexts["3 stocks selected"].waitForExistence(timeout: 10), "unstarring did not count")
        attachScreenshot(app, name: "watchlist-toggled")
    }

    @MainActor
    func testAnEmptyWatchlistInvitesStars() throws {
        let app = launch("empty")
        waitForScreen(app, "empty")

        XCTAssertTrue(app.staticTexts["No stocks selected"].waitForExistence(timeout: 25), "the count never drew")
        XCTAssertTrue(app.staticTexts["Your watchlist is empty"].exists, "the empty state never drew")
        XCTAssertTrue(app.staticTexts["Add stocks using the stars below."].exists)
        XCTAssertTrue(anyElement(app, "watchlist-row-NVDAx").waitForExistence(timeout: 25), "the catalogue is missing")
    }

    @MainActor
    func testASearchWithNoMatchSaysSo() throws {
        let app = launch("noResults")
        waitForScreen(app, "noResults")

        XCTAssertTrue(app.staticTexts["No matching stocks"].waitForExistence(timeout: 25), "no empty answer")
        XCTAssertTrue(app.staticTexts["Try another search."].exists)
        XCTAssertTrue(app.staticTexts["No stocks selected"].exists, "the count line stays")
    }

    @MainActor
    func testAFailedReadOffersRetry() throws {
        let app = launch("failed")
        waitForScreen(app, "failed")

        XCTAssertTrue(app.staticTexts["Couldn't load this view"].waitForExistence(timeout: 25), "no failure state")
        XCTAssertEqual(app.buttons["watchlist-failed"].label, "Try again")
        XCTAssertFalse(app.textFields["monaco-search-field"].exists, "a full-screen error has no search field")
    }

    @MainActor
    func testTheRowsSurviveAccessibilityTextSizes() throws {
        let app = launch("ready", textSize: "UICTContentSizeCategoryAccessibilityXXXL")
        waitForScreen(app, "ready at AX5")

        XCTAssertTrue(anyElement(app, "watchlist-row-NVDAx").waitForExistence(timeout: 30), "no row at AX5")
        XCTAssertTrue(app.buttons["watchlist-star-NVDAx"].exists, "the star should survive AX5")
        attachScreenshot(app, name: "watchlist-ax5")
    }
}
