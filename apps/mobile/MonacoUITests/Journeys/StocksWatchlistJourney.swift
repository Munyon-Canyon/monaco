import XCTest

enum StocksWatchlistJourney {
    static let id = "stocks/watchlist"
    static let version = 2

    static let alpha = StocksBrowseJourney.alpha
    static let starred = "watchlist-star-\(alpha.symbol)"
    static let marketsRow = "markets-watchlist-row-\(alpha.symbol)"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func goBack(_ app: XCUIApplication) {
        app.navigationBars.buttons.element(boundBy: 0).tap()
    }

    static func waitForLabel(_ element: XCUIElement, _ label: String, timeout: TimeInterval) -> Bool {
        let matches = NSPredicate(format: "label == %@", label)
        let waiter = XCTNSPredicateExpectation(predicate: matches, object: element)
        return XCTWaiter().wait(for: [waiter], timeout: timeout) == .completed
    }

    static func starFromManage(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Markets tab with an empty Watchlist") {
            StocksBrowseJourney.openStocks(app, step: "S1.1")
            let add = app.element("markets-watchlist-add")
            app.scrollIntoReach(add)
            XCTAssertTrue(add.waitForExistence(timeout: 15), "S1.1: no Add button in the empty Watchlist section")
            XCTAssertTrue(
                app.staticTexts["Your market, at a glance"].exists, "S1.1: no empty Watchlist card copy")
            XCTAssertTrue(
                app.element("markets-watchlist-manage").exists, "S1.1: no star round in the Markets bar")
        }

        recorder.step("S1.2", "open the watchlist from the star round") {
            let manage = app.element("markets-watchlist-manage")
            XCTAssertTrue(manage.waitForExistence(timeout: 10), "S1.2: no star round in the Markets bar")
            manage.tap()
            XCTAssertTrue(
                app.element("watchlist-root").waitForExistence(timeout: 10), "S1.2: Your watchlist did not open")
            XCTAssertTrue(
                app.navigationBars["Your watchlist"].exists, "S1.2: the screen is not titled 'Your watchlist'")
            XCTAssertTrue(
                waitForLabel(app.element("watchlist-count"), "No stocks selected", timeout: 10),
                "S1.2: the count line reads '\(app.element("watchlist-count").label)', not 'No stocks selected'")
        }

        recorder.step("S1.3", "search for the stock") {
            StocksBrowseJourney.replaceQuery(app, with: alpha.ticker)
            let row = app.element("watchlist-row-\(alpha.symbol)")
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.3: no watchlist-row-\(alpha.symbol) within 15 s")
            XCTAssertTrue(row.label.contains(alpha.name), "S1.3: the row reads '\(row.label)', not '\(alpha.name)'")
            XCTAssertTrue(row.label.contains(alpha.ticker), "S1.3: the row reads '\(row.label)', not '\(alpha.ticker)'")
            XCTAssertEqual(
                app.element(starred).label, "Add \(alpha.name)", "S1.3: the unstarred row's star is mislabelled")
        }

        recorder.step("S1.4", "star it") {
            app.element(starred).tap()
            XCTAssertTrue(
                waitForLabel(app.element(starred), "Remove \(alpha.name)", timeout: 5),
                "S1.4: the star reads '\(app.element(starred).label)', not 'Remove \(alpha.name)'")
            XCTAssertTrue(
                waitForLabel(app.element("watchlist-count"), "1 stock selected", timeout: 5),
                "S1.4: the count line reads '\(app.element("watchlist-count").label)', not '1 stock selected'")
        }

        recorder.step("S1.5", "go back and see it in the Watchlist") {
            goBack(app)
            let row = app.element(marketsRow)
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.5: no \(marketsRow) on the Markets tab")
            XCTAssertTrue(row.label.contains(alpha.ticker), "S1.5: the row reads '\(row.label)', not '\(alpha.ticker)'")
            XCTAssertFalse(app.element("markets-watchlist-add").exists, "S1.5: the empty Watchlist card is still there")
        }
    }

    static func removeFromStock(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let star = app.element("asset-watch-star")

        recorder.step("S2.1", "open the stock from the Watchlist") {
            let row = app.element(marketsRow)
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S2.1: no \(marketsRow) on the Markets tab")
            row.tap()
            XCTAssertTrue(
                app.element("asset-detail-root").waitForExistence(timeout: 15), "S2.1: the stock did not open")
            XCTAssertEqual(
                app.element("asset-detail-name").label, alpha.name, "S2.1: the stock screen is not \(alpha.name)")
            XCTAssertTrue(
                waitForLabel(star, "Remove from watchlist", timeout: 10),
                "S2.1: the star reads '\(star.label)', not 'Remove from watchlist'")
        }

        recorder.step("S2.2", "tap the star") {
            star.tap()
            XCTAssertTrue(
                waitForLabel(star, "Add to watchlist", timeout: 5),
                "S2.2: the star reads '\(star.label)', not 'Add to watchlist'")
            XCTAssertFalse(app.element("monaco-toast-banner").exists, "S2.2: a toast showed for the star")
        }

        recorder.step("S2.3", "go back to an empty Watchlist") {
            goBack(app)
            let add = app.element("markets-watchlist-add")
            app.scrollIntoReach(add)
            XCTAssertTrue(add.waitForExistence(timeout: 10), "S2.3: the empty Watchlist card did not come back")
            XCTAssertFalse(app.element(marketsRow).exists, "S2.3: \(marketsRow) is still in the Watchlist")
        }
    }
}
