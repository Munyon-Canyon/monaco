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
        let field = app.textFields["monaco-search-field"]
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
    private func chip(_ app: XCUIApplication, _ title: String) -> XCUIElement {
        anyElement(app, "markets-chip-\(title)")
    }

    @MainActor
    private func waitForAbsence(of element: XCUIElement, timeout: TimeInterval = 25) -> Bool {
        let gone = expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: element)
        return XCTWaiter().wait(for: [gone], timeout: timeout) == .completed
    }

    @MainActor
    func testEveryScenarioDraws() throws {
        let scenarios = [
            "ready", "watchlistEmpty", "noResults", "loading", "stale", "failed", "empty", "paused", "movers", "held",
        ]
        for scenario in scenarios {
            let app = launch(scenario)
            if scenario == "noResults" {
                _ = waitForSearchField(app, scenario)
            } else {
                waitForTab(app, scenario)
            }
            attachScreenshot(app, name: "markets-\(scenario)")
            app.terminate()
        }
    }

    @MainActor
    func testReadyShowsTheSectionsInOrder() throws {
        let app = launch("ready")
        waitForTab(app, "ready")

        _ = waitForSearchField(app, "ready")
        XCTAssertEqual(app.textFields["monaco-search-field"].placeholderValue, "Stocks and Pre-IPO")
        for title in ["All", "Watchlist", "Pre-IPO", "Held"] {
            XCTAssertTrue(chip(app, title).exists, "no \(title) chip")
        }
        let watchlist = anyElement(app, "markets-watchlist")
        XCTAssertTrue(watchlist.waitForExistence(timeout: 25), "no Watchlist section")
        XCTAssertTrue(anyElement(app, "markets-watchlist-row-AAPLx").waitForExistence(timeout: 25), "no watched row")
        let preIpo = anyElement(app, "markets-preipo")
        scrollTo(app, preIpo, "Pre-IPO")
        let movers = anyElement(app, "markets-movers")
        scrollTo(app, movers, "Moving today")
        XCTAssertLessThan(watchlist.frame.minY, preIpo.frame.minY, "Watchlist should lead Pre-IPO")
        XCTAssertLessThan(preIpo.frame.minY, movers.frame.minY, "Pre-IPO should lead Moving today")
    }

    @MainActor
    func testTheWatchlistShowsFiveRowsAndSeeAllShowsEveryRow() throws {
        let app = launch("ready")
        waitForTab(app, "ready")

        XCTAssertTrue(anyElement(app, "markets-watchlist-row-AMZNx").waitForExistence(timeout: 25), "no fifth row")
        XCTAssertFalse(anyElement(app, "markets-watchlist-row-MSFTx").exists, "the section previews five rows")
        let seeAll = anyElement(app, "markets-watchlist-see-all")
        XCTAssertTrue(seeAll.exists, "no See all link")
        seeAll.tap()

        XCTAssertTrue(anyElement(app, "markets-watchlist-row-MSFTx").waitForExistence(timeout: 25), "See all lost rows")
        XCTAssertTrue(anyElement(app, "markets-watchlist-row-tSpaceX").exists, "See all should list Pre-IPO rows")
        XCTAssertFalse(anyElement(app, "markets-watchlist-see-all").exists, "the Watchlist chip has no See all")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "markets-movers")), "the Watchlist chip drops other sections")
    }

    @MainActor
    func testChipsFilterTheList() throws {
        let app = launch("ready")
        waitForTab(app, "ready")
        XCTAssertTrue(anyElement(app, "markets-watchlist-row-NVDAx").waitForExistence(timeout: 25), "no rows")

        chip(app, "Pre-IPO").tap()
        XCTAssertTrue(anyElement(app, "markets-preipo-row-tSpaceX").waitForExistence(timeout: 25), "no SpaceX")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "markets-watchlist")), "Pre-IPO should not list the Watchlist")
        XCTAssertFalse(anyElement(app, "markets-preipo-see-all").exists, "the Pre-IPO chip has no See all")
        XCTAssertFalse(anyElement(app, "markets-movers").exists, "the Pre-IPO chip has no Moving today")

        chip(app, "Watchlist").tap()
        XCTAssertTrue(anyElement(app, "markets-watchlist-row-TSLAx").waitForExistence(timeout: 25), "no Watchlist rows")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "markets-preipo")), "Watchlist should not list Pre-IPO")

        chip(app, "All").tap()
        XCTAssertTrue(anyElement(app, "markets-preipo").waitForExistence(timeout: 25), "All lost the Pre-IPO section")
        scrollTo(app, anyElement(app, "markets-movers"), "Moving today")
    }

    @MainActor
    func testTheHeldChipListsWhatCabalsHold() throws {
        let app = launch("held")
        waitForTab(app, "held")

        XCTAssertTrue(anyElement(app, "markets-held").waitForExistence(timeout: 25), "the Held list never drew")
        XCTAssertTrue(anyElement(app, "markets-held-total").exists, "no held total")
        XCTAssertFalse(anyElement(app, "markets-watchlist").exists, "Held should not list the Watchlist")

        chip(app, "All").tap()
        XCTAssertTrue(anyElement(app, "markets-watchlist").waitForExistence(timeout: 25), "All lost the Watchlist")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "markets-held")), "All should not list Held")
    }

    @MainActor
    func testMovingTodayKeepsTheBiggestMoveFirst() throws {
        let app = launch("movers")
        waitForTab(app, "movers")

        let amd = anyElement(app, "markets-mover-AMDx")
        scrollTo(app, amd, "the AMD tile")
        let netflix = anyElement(app, "markets-mover-NFLXx")
        let tesla = anyElement(app, "markets-mover-TSLAx")
        XCTAssertTrue(netflix.waitForExistence(timeout: 10), "no Netflix tile")
        XCTAssertTrue(tesla.waitForExistence(timeout: 10), "no Tesla tile")
        XCTAssertLessThan(amd.frame.minX, netflix.frame.minX, "AMD (+7.9%) should lead Netflix (+5.2%)")
        XCTAssertLessThan(netflix.frame.minX, tesla.frame.minX, "Netflix (+5.2%) should lead Tesla (-2.4%)")
        XCTAssertTrue(amd.label.hasPrefix("AMD"), "the tile should name AMD, got \(amd.label)")

        amd.swipeLeft()
        let amazon = anyElement(app, "markets-mover-AMZNx")
        let microsoft = anyElement(app, "markets-mover-MSFTx")
        XCTAssertTrue(amazon.waitForExistence(timeout: 10), "the rail never reached Amazon")
        XCTAssertTrue(microsoft.waitForExistence(timeout: 10), "the fifth tile is missing")
        XCTAssertLessThan(amazon.frame.minX, microsoft.frame.minX, "Amazon (-2.1%) should lead Microsoft (+1.6%)")
        XCTAssertFalse(anyElement(app, "markets-mover-NVDAx").exists, "the rail holds five tiles")
    }

    @MainActor
    func testAnEmptyWatchlistInvitesTheFirstStar() throws {
        let app = launch("watchlistEmpty")
        waitForTab(app, "watchlistEmpty")

        XCTAssertTrue(app.staticTexts["Your market, at a glance"].waitForExistence(timeout: 25), "no glance card")
        XCTAssertTrue(app.staticTexts["Star a stock to watch it here."].exists, "no invitation")
        XCTAssertFalse(anyElement(app, "markets-watchlist-see-all").exists, "an empty watchlist has no See all")
        app.buttons["markets-watchlist-add"].tap()

        XCTAssertTrue(app.navigationBars["Your watchlist"].waitForExistence(timeout: 25), "Add did not open it")
    }

    @MainActor
    func testSearchReplacesBrowsingWithResultsAndComesBack() throws {
        let app = launch("ready")
        waitForTab(app, "ready")
        XCTAssertTrue(anyElement(app, "markets-watchlist").waitForExistence(timeout: 25), "the tab never drew")

        let field = waitForSearchField(app, "ready")
        field.tap()
        field.typeText("Alphabet")

        XCTAssertTrue(anyElement(app, "assets-row-GOOGLx").waitForExistence(timeout: 25), "search results never drew")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "assets-row-NVDAx")), "NVIDIA does not match Alphabet")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "markets-watchlist")), "search mode drops the browse sections")

        anyElement(app, "markets-search-cancel").tap()
        XCTAssertTrue(anyElement(app, "markets-watchlist").waitForExistence(timeout: 25), "browsing never came back")
    }

    @MainActor
    func testASearchWithNoMatchSaysSo() throws {
        let app = launch("noResults")
        _ = waitForSearchField(app, "noResults")

        XCTAssertTrue(app.staticTexts["No results for “zzzz”"].waitForExistence(timeout: 25), "no empty answer")
        XCTAssertTrue(app.staticTexts["Try a company name or ticker."].exists)
        XCTAssertFalse(anyElement(app, "markets-watchlist").exists, "search mode drops the browse sections")
    }

    @MainActor
    func testAFailedRefreshKeepsTheRowsAndSaysPricesAreDelayed() throws {
        let app = launch("stale")
        waitForTab(app, "stale")

        XCTAssertTrue(anyElement(app, "markets-watchlist-row-NVDAx").waitForExistence(timeout: 25), "no rows")
        let notice = anyElement(app, "markets-stale")
        XCTAssertTrue(notice.waitForExistence(timeout: 30), "no stale notice")
        XCTAssertTrue(
            notice.label.contains("Prices delayed · just now.") || app.staticTexts["Prices delayed · just now."].exists,
            "the notice copy is not the spec's"
        )
        XCTAssertTrue(anyElement(app, "markets-watchlist-row-NVDAx").exists, "the rows must stay")
        attachScreenshot(app, name: "markets-stale")
    }

    @MainActor
    func testAFailedCatalogueIsTheFullScreenErrorWithRetry() throws {
        let app = launch("failed")
        waitForTab(app, "failed")

        XCTAssertTrue(app.staticTexts["Couldn't load this view"].waitForExistence(timeout: 25), "no failure state")
        XCTAssertEqual(app.buttons["assets-failed"].label, "Try again")
        XCTAssertFalse(app.textFields["monaco-search-field"].exists, "a full-screen error has no search field")
        XCTAssertFalse(anyElement(app, "markets-watchlist").exists, "a failed read must not show sections")
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

        let first = anyElement(app, "markets-mover-SBUXx")
        scrollTo(app, first, "the paused Starbucks tile")
        XCTAssertTrue(first.label.contains("today"), "got \(first.label)")
        first.swipeLeft()
        let tile = anyElement(app, "markets-mover-NEWCO")
        XCTAssertTrue(tile.waitForExistence(timeout: 10), "the unpriced Newco tile never drew")
        XCTAssertTrue(tile.label.hasSuffix("today unavailable"), "Newco has no move, got \(tile.label)")
    }

    @MainActor
    func testTheTabSurvivesAccessibilityTextSizes() throws {
        let app = launch("ready", textSize: "UICTContentSizeCategoryAccessibilityXXXL")
        waitForTab(app, "ready at AX5")

        XCTAssertTrue(
            anyElement(app, "markets-watchlist-row-NVDAx").waitForExistence(timeout: 30),
            "the rows should stack at AX5"
        )
        XCTAssertTrue(app.textFields["monaco-search-field"].exists, "the search field should survive AX5")
        XCTAssertEqual(app.textFields["monaco-search-field"].placeholderValue, "Search")
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
