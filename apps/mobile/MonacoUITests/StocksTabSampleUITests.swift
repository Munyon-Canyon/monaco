import XCTest

nonisolated final class StocksTabSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String, textSize: String? = nil) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoStocksTabSample", scenario]
        if let textSize {
            app.launchArguments += ["-UIPreferredContentSizeCategoryName", textSize]
        }
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
    private func anyElement(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func waitForTab(_ app: XCUIApplication, _ context: String) {
        XCTAssertTrue(
            app.navigationBars["Markets"].waitForExistence(timeout: 30),
            "\(context) never drew the Markets tab"
        )
    }

    @MainActor
    private func waitForSearchField(_ app: XCUIApplication, _ context: String) -> XCUIElement {
        let field = app.textFields["monaco-search-field"].firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 25), "\(context) never drew the search field")
        return field
    }

    @MainActor
    private func scrollTo(_ app: XCUIApplication, _ element: XCUIElement, _ context: String) {
        for _ in 0..<8 where !(element.exists && element.isHittable) {
            app.swipeUp()
        }
        XCTAssertTrue(element.exists, "\(context) never scrolled into view")
    }

    @MainActor
    private func waitForAbsence(of element: XCUIElement, timeout: TimeInterval = 25) -> Bool {
        let gone = expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: element)
        return XCTWaiter().wait(for: [gone], timeout: timeout) == .completed
    }

    @MainActor
    func testEveryScenarioDraws() throws {
        let scenarios = ["ready", "watchlistEmpty", "noResults", "loading", "stale", "failed", "empty", "paused"]
        for scenario in scenarios {
            let app = launch(scenario)
            waitForTab(app, scenario)
            attachScreenshot(app, name: "markets-\(scenario)")
            app.terminate()
        }
    }

    @MainActor
    func testReadyShowsTheSectionsInOrder() throws {
        let app = launch("ready")
        waitForTab(app, "ready")

        _ = waitForSearchField(app, "ready")
        XCTAssertEqual(app.textFields["monaco-search-field"].placeholderValue, "Search stocks or tickers")
        XCTAssertTrue(anyElement(app, "markets-mover-NVDAx").waitForExistence(timeout: 25), "no mover tile")
        XCTAssertTrue(anyElement(app, "markets-watchlist-row-AAPLx").waitForExistence(timeout: 25), "no watched row")
        XCTAssertTrue(app.buttons["markets-watchlist-manage"].exists, "no Manage link")
        attachScreenshot(app, name: "markets-ready-top")
        scrollTo(app, anyElement(app, "markets-popular-row-NVDAx"), "Popular now")
        scrollTo(app, anyElement(app, "assets-row-NVDAx"), "All stocks")
        XCTAssertTrue(anyElement(app, "stocks-sort").exists, "no Sort select")
        XCTAssertTrue(anyElement(app, "stocks-filter").exists, "no Filter select")
        attachScreenshot(app, name: "markets-ready-bottom")
    }

    @MainActor
    func testMoversAreOrderedByTheBiggestDayMove() throws {
        let app = launch("ready")
        waitForTab(app, "ready")

        let nvidia = anyElement(app, "markets-mover-NVDAx")
        XCTAssertTrue(nvidia.waitForExistence(timeout: 25), "no mover tile")
        XCTAssertLessThan(nvidia.frame.minX, anyElement(app, "markets-mover-AMDx").frame.minX, "NVDA should lead AMD")
        nvidia.swipeLeft()
        let tesla = anyElement(app, "markets-mover-TSLAx")
        XCTAssertTrue(tesla.waitForExistence(timeout: 10), "the rail never reached Tesla")
        let apple = anyElement(app, "markets-mover-AAPLx")
        XCTAssertTrue(apple.waitForExistence(timeout: 10), "the fourth tile is missing")
        XCTAssertLessThan(tesla.frame.minX, apple.frame.minX, "TSLA should lead AAPL")
        XCTAssertFalse(anyElement(app, "markets-mover-MSFTx").exists, "the rail holds four tiles")
    }

    @MainActor
    func testManageOpensTheWatchlist() throws {
        let app = launch("ready")
        waitForTab(app, "ready")

        let manage = app.buttons["markets-watchlist-manage"]
        XCTAssertTrue(manage.waitForExistence(timeout: 25), "no Manage link")
        manage.tap()

        XCTAssertTrue(app.navigationBars["Your watchlist"].waitForExistence(timeout: 25), "Manage did not open it")
    }

    @MainActor
    func testAnEmptyWatchlistInvitesTheFirstStar() throws {
        let app = launch("watchlistEmpty")
        waitForTab(app, "watchlistEmpty")

        XCTAssertTrue(app.staticTexts["Your market, at a glance"].waitForExistence(timeout: 25), "no glance card")
        XCTAssertTrue(app.staticTexts["Add stocks to your watchlist."].exists)
        app.buttons["markets-watchlist-add"].tap()

        XCTAssertTrue(app.navigationBars["Your watchlist"].waitForExistence(timeout: 25), "Add did not open it")
        attachScreenshot(app, name: "markets-watchlist-empty-add")
    }

    @MainActor
    func testSearchReplacesBrowsingWithResultsAndComesBack() throws {
        let app = launch("ready")
        waitForTab(app, "ready")
        XCTAssertTrue(anyElement(app, "markets-mover-NVDAx").waitForExistence(timeout: 25), "the tab never drew")

        let field = waitForSearchField(app, "ready")
        field.tap()
        field.typeText("Alphabet")

        XCTAssertTrue(app.staticTexts["Search results"].waitForExistence(timeout: 25), "no results header")
        XCTAssertTrue(anyElement(app, "assets-row-GOOGLx").waitForExistence(timeout: 25), "search results never drew")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "assets-row-NVDAx")), "NVIDIA does not match Alphabet")
        XCTAssertFalse(app.staticTexts["Top movers"].exists, "search mode drops the browse sections")

        app.buttons["Clear search"].firstMatch.tap()
        XCTAssertTrue(anyElement(app, "markets-mover-NVDAx").waitForExistence(timeout: 25), "browsing never came back")
    }

    @MainActor
    func testASearchWithNoMatchSaysSo() throws {
        let app = launch("noResults")
        waitForTab(app, "noResults")

        XCTAssertTrue(app.staticTexts["No matching stocks"].waitForExistence(timeout: 25), "no empty answer")
        XCTAssertTrue(app.staticTexts["Try a company name, ticker or another filter."].exists)
        XCTAssertTrue(app.staticTexts["Search results"].exists)
        attachScreenshot(app, name: "markets-no-results")
    }

    @MainActor
    func testTheFilterNarrowsTheList() throws {
        let app = launch("ready")
        waitForTab(app, "ready")
        let filter = anyElement(app, "stocks-filter")
        scrollTo(app, filter, "the Filter select")

        filter.tap()
        app.buttons["Pre-IPO"].tap()

        XCTAssertTrue(anyElement(app, "assets-row-tSpaceX").waitForExistence(timeout: 25), "Pre-IPO lost its row")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "assets-row-NVDAx")), "Pre-IPO should not list NVIDIA")
        attachScreenshot(app, name: "markets-filter-pre-ipo")
    }

    @MainActor
    func testOnlyTheDefaultSortCanBePicked() throws {
        let app = launch("ready")
        waitForTab(app, "ready")
        let sort = anyElement(app, "stocks-sort")
        scrollTo(app, sort, "the Sort select")

        sort.tap()

        XCTAssertTrue(app.buttons["Default"].waitForExistence(timeout: 10), "no Default option")
        XCTAssertTrue(app.staticTexts["More sorts open soon"].exists, "no explanation for the other sorts")
        for title in ["Volume", "Trade count", "Name A–Z", "Price change"] {
            XCTAssertFalse(app.buttons[title].isEnabled, "\(title) should be disabled")
        }
        attachScreenshot(app, name: "markets-sort-menu")
    }

    @MainActor
    func testAFailedRefreshKeepsTheRowsAndSaysPricesAreDelayed() throws {
        let app = launch("stale")
        waitForTab(app, "stale")

        XCTAssertTrue(anyElement(app, "markets-mover-NVDAx").waitForExistence(timeout: 25), "the rows never drew")
        let notice = anyElement(app, "markets-stale")
        XCTAssertTrue(notice.waitForExistence(timeout: 30), "no stale notice")
        XCTAssertTrue(app.staticTexts["Prices delayed · just now."].exists, "the notice copy is not the spec's")
        XCTAssertTrue(anyElement(app, "markets-mover-NVDAx").exists, "the rows must stay")
        attachScreenshot(app, name: "markets-stale")
    }

    @MainActor
    func testAFailedCatalogueIsTheFullScreenErrorWithRetry() throws {
        let app = launch("failed")
        waitForTab(app, "failed")

        XCTAssertTrue(app.staticTexts["Couldn't load this view"].waitForExistence(timeout: 25), "no failure state")
        XCTAssertEqual(app.buttons["assets-failed"].label, "Try again")
        XCTAssertFalse(app.textFields["monaco-search-field"].exists, "a full-screen error has no search field")
        XCTAssertFalse(anyElement(app, "assets-row-NVDAx").exists, "a failed read must not show rows")
    }

    @MainActor
    func testAnEmptyCatalogueIsAnEmptyStateNotAnError() throws {
        let app = launch("empty")
        waitForTab(app, "empty")

        XCTAssertTrue(app.staticTexts["No stocks to show"].waitForExistence(timeout: 25), "the empty state never drew")
        XCTAssertFalse(app.staticTexts["Couldn't load this view"].exists, "an empty catalogue is not a failure")
        XCTAssertTrue(app.textFields["monaco-search-field"].exists, "the search field should stay live")
    }

    @MainActor
    func testAStockWithoutAPriceStillRenders() throws {
        let app = launch("paused")
        waitForTab(app, "paused")

        scrollTo(app, anyElement(app, "assets-row-NEWCO"), "the unpriced Newco row")
    }

    @MainActor
    func testTheTabSurvivesAccessibilityTextSizes() throws {
        let app = launch("ready", textSize: "UICTContentSizeCategoryAccessibilityXXXL")
        waitForTab(app, "ready at AX5")

        XCTAssertTrue(
            anyElement(app, "markets-mover-NVDAx").waitForExistence(timeout: 30),
            "the rail should stack at AX5"
        )
        XCTAssertTrue(app.textFields["monaco-search-field"].exists, "the search field should survive AX5")
        attachScreenshot(app, name: "markets-ax5")
    }

    @MainActor
    func testTappingARowOpensTheStock() throws {
        let app = launch("ready")
        waitForTab(app, "ready")

        let row = anyElement(app, "markets-watchlist-row-AAPLx")
        XCTAssertTrue(row.waitForExistence(timeout: 25), "the Apple row is missing")
        row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()

        XCTAssertTrue(waitForAbsence(of: app.navigationBars["Markets"]), "the tab was never left")
    }
}
