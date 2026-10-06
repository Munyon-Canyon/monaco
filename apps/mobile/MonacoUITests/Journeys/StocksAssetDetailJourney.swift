import XCTest

enum StocksAssetDetailJourney {
    static let id = "stocks/asset-detail"
    static let version = 2

    static let alpha = StocksBrowseJourney.alpha
    static let preIpo = StocksBrowseJourney.preIpo
    static let otherListingID = "asset-other-listing-JRNYQx"
    static let otherListingTicker = "JRNYQx"
    static let buyCaption = "Your cabal votes before anything is bought"
    static let ranges = ["1D", "1W", "1M", "3M", "1Y", "ALL"]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func openAsset(
        _ app: XCUIApplication, _ asset: StocksBrowseJourney.SeededAsset, scrolls: Bool, step: String
    ) {
        StocksBrowseJourney.openStocks(app, step: step)
        let row = StocksBrowseJourney.row(app, asset)
        if scrolls {
            XCTAssertTrue(
                StocksBrowseJourney.scrollTo(app, row, timeout: 15),
                "\(step): no \(asset.rowID) within 15 s (the catalogue did not list it)")
        } else {
            StocksBrowseJourney.waitForRow(app, asset, step: step, timeout: 15)
        }
        row.tap()
        XCTAssertTrue(
            app.element("asset-detail-root").waitForExistence(timeout: 15), "\(step): the asset screen did not show")
    }

    static func cabalRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH 'propose-pick-cabal-'"))
    }

    static func scrollToText(_ app: XCUIApplication, _ text: String, step: String) {
        let element = app.staticTexts[text]
        var swipes = 0
        while !element.exists && swipes < 6 {
            app.element("asset-detail-root").swipeUp()
            swipes += 1
        }
        XCTAssertTrue(element.waitForExistence(timeout: 10), "\(step): no '\(text)' on the asset screen (#577)")
    }

    static func rangeChange(_ app: XCUIApplication) -> String {
        app.staticTexts.matching(identifier: "asset-detail-range-change").allElementsBoundByIndex
            .map(\.label).joined(separator: " ")
    }

    static func waitForRangeChange(_ app: XCUIApplication, _ text: String, timeout: TimeInterval) -> Bool {
        let match = app.staticTexts.matching(identifier: "asset-detail-range-change")
            .matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch
        return match.waitForExistence(timeout: timeout)
    }

    static func heroChartAndBuy(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open Journey Alpha") {
            openAsset(app, alpha, scrolls: false, step: "S1.1")
            let title = app.navigationBars[alpha.ticker]
            XCTAssertTrue(title.exists, "S1.1: the asset screen is not titled '\(alpha.ticker)'")
        }

        recorder.step("S1.2", "read the hero") {
            XCTAssertEqual(app.staticTexts["asset-detail-name"].label, alpha.name, "S1.2: the name is not the seed's")
            let price = app.staticTexts["asset-detail-price"].label
            XCTAssertEqual(price, alpha.price, "S1.2: the price is not the seed's")
            XCTAssertTrue(app.staticTexts["Trading 24/7 on Solana"].exists, "S1.2: no 'Trading 24/7 on Solana'")
        }

        recorder.step("S1.3", "read the range chips") {
            XCTAssertTrue(
                waitForRangeChange(app, "Past day · \(alpha.ticker)", timeout: 15),
                "S1.3: the change chip reads '\(rangeChange(app))', not 'Past day · \(alpha.ticker)'")
            let chips = app.element("asset-chart-ranges")
            for range in ranges {
                XCTAssertTrue(chips.buttons[range].exists, "S1.3: no '\(range)' range chip")
            }
            XCTAssertTrue(chips.buttons["1D"].isSelected, "S1.3: '1D' is not selected")
        }

        recorder.step("S1.4", "see the chart") {
            let chart = app.element("asset-detail-chart")
            XCTAssertTrue(chart.waitForExistence(timeout: 15), "S1.4: no chart for the seeded samples")
        }

        recorder.step("S1.5", "pick one week") {
            let week = app.element("asset-chart-ranges").buttons["1W"]
            week.tap()
            XCTAssertTrue(
                waitForRangeChange(app, "Past week · \(alpha.ticker)", timeout: 10),
                "S1.5: the change chip reads '\(rangeChange(app))', not 'Past week · \(alpha.ticker)'")
            XCTAssertTrue(week.isSelected, "S1.5: '1W' is not selected")
        }

        recorder.step("S1.6", "pick one year") {
            let year = app.element("asset-chart-ranges").buttons["1Y"]
            year.tap()
            XCTAssertTrue(
                waitForRangeChange(app, "Past year · \(alpha.ticker)", timeout: 10),
                "S1.6: the change chip reads '\(rangeChange(app))', not 'Past year · \(alpha.ticker)'")
            XCTAssertTrue(year.isSelected, "S1.6: '1Y' is not selected")
        }

        recorder.step("S1.7", "read Propose buy") {
            let buy = app.buttons["asset-detail-propose-buy"]
            XCTAssertTrue(buy.waitForExistence(timeout: 15), "S1.7: no Propose buy")
            XCTAssertEqual(buy.label, "Propose buy", "S1.7: the CTA reads '\(buy.label)', not 'Propose buy'")
            XCTAssertTrue(buy.isEnabled, "S1.7: Propose buy is disabled for a tradable asset")
            XCTAssertTrue(app.staticTexts[buyCaption].exists, "S1.7: no '\(buyCaption)' under Propose buy")
        }

        recorder.step("S1.8", "tap Propose buy") {
            app.buttons["asset-detail-propose-buy"].tap()
            let title = "Which cabal should buy \(alpha.ticker)?"
            XCTAssertTrue(
                app.navigationBars[title].waitForExistence(timeout: 10),
                "S1.8: Propose buy did not open the cabal picker titled '\(title)' (#613)")
            XCTAssertTrue(cabalRows(app).firstMatch.waitForExistence(timeout: 10), "S1.8: no cabal row in the picker")
            XCTAssertGreaterThanOrEqual(cabalRows(app).count, 2, "S1.8: the picker lists fewer than two cabals")
        }

        recorder.step("S1.9", "pick a cabal and reach Amount") {
            cabalRows(app).firstMatch.tap()
            XCTAssertTrue(
                app.element("propose-amount-screen").waitForExistence(timeout: 10),
                "S1.9: the cabal row did not open the Amount screen (#613)")
            XCTAssertTrue(app.navigationBars["Amount"].exists, "S1.9: the screen is not titled 'Amount'")
            XCTAssertTrue(app.staticTexts[alpha.ticker].exists, "S1.9: no \(alpha.ticker) row on Amount")
        }
    }

    static func stats(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open Journey Alpha") {
            openAsset(app, alpha, scrolls: false, step: "S2.1")
        }

        recorder.step("S2.2", "scroll to Stats") {
            scrollToText(app, "Stats", step: "S2.2")
        }

        recorder.step("S2.3", "see the 52-week bar") {
            scrollToText(app, "52-week range", step: "S2.3")
        }
    }

    static func preIpoBlock(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open Journey Private") {
            openAsset(app, preIpo, scrolls: true, step: "S3.1")
            XCTAssertEqual(
                app.staticTexts["asset-detail-name"].label, preIpo.name, "S3.1: the name is not '\(preIpo.name)'")
        }

        recorder.step("S3.2", "see Also available from") {
            scrollToText(app, "Also available from", step: "S3.2")
            XCTAssertTrue(app.element(otherListingID).exists, "S3.2: no \(otherListingID) under 'Also available from'")
        }

        recorder.step("S3.3", "open the other listing") {
            let listing = app.element(otherListingID)
            let bar = app.buttons["asset-detail-propose-buy"]
            var swipes = 0
            while listing.frame.midY > bar.frame.minY - 20 && swipes < 4 {
                let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.78))
                from.press(
                    forDuration: 0.1, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3)))
                swipes += 1
            }
            listing.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.2)).tap()
            XCTAssertTrue(
                app.navigationBars[otherListingTicker].waitForExistence(timeout: 10),
                "S3.3: the listing did not open an asset screen titled '\(otherListingTicker)')")
            XCTAssertTrue(app.element("asset-detail-root").exists, "S3.3: no asset screen for the other listing")
        }
    }

    static func position(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S4.1", "open Journey Alpha") {
            openAsset(app, alpha, scrolls: false, step: "S4.1")
        }

        recorder.step("S4.2", "see Your cabals' position") {
            scrollToText(app, "Your cabals' position", step: "S4.2")
            let holding = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'asset-position-holding-'")).firstMatch
            XCTAssertTrue(holding.exists, "S4.2: no cabal row under 'Your cabals' position' (#2136)")
        }

        recorder.step("S4.3", "tap Propose sell") {
            let sell = app.buttons["asset-detail-sell"]
            XCTAssertTrue(sell.waitForExistence(timeout: 5), "S4.3: no Propose sell (#577)")
            sell.tap()
            XCTAssertTrue(
                app.navigationBars["Propose sell"].waitForExistence(timeout: 10),
                "S4.3: Propose sell did not open the propose screen")
        }
    }
}
