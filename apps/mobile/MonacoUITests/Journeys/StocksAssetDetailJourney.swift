import XCTest

enum StocksAssetDetailJourney {
    static let id = "stocks/asset-detail"
    static let version = 11

    static let alpha = StocksBrowseJourney.alpha
    static let preIpo = StocksBrowseJourney.preIpo
    static let otherListingID = "asset-other-listing-JRNYQx"
    static let sourceListingID = "asset-other-listing-JRNYPx"
    static let buyCaption = "Your cabal votes before anything is bought"
    static let ranges = ["1D", "1W", "1M", "3M", "6M", "1Y", "ALL"]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func rangeChip(_ app: XCUIApplication, _ range: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(
                format: "(identifier == 'asset-chart-ranges' OR identifier == %@) AND label == %@",
                "asset-range-\(range)", range)
        ).firstMatch
    }

    static func openAsset(_ app: XCUIApplication, _ asset: StocksBrowseJourney.SeededAsset, step: String) {
        StocksBrowseJourney.openStocks(app, step: step)
        XCTAssertTrue(
            StocksBrowseJourney.searchField(app).waitForExistence(timeout: 15),
            "\(step): no search field on the Markets tab")
        StocksBrowseJourney.replaceQuery(app, with: asset.ticker)
        let row = StocksBrowseJourney.row(app, asset)
        XCTAssertTrue(
            row.waitForExistence(timeout: 15),
            "\(step): no \(asset.rowID) within 15 s of searching '\(asset.ticker)'")
        row.tap()
        XCTAssertTrue(
            app.element("asset-detail-root").waitForExistence(timeout: 15), "\(step): the asset screen did not show")
    }

    static func cabalRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(
            NSPredicate(
                format: "identifier BEGINSWITH 'propose-pick-cabal-' AND identifier != 'propose-pick-cabal-question'"))
    }

    static func expectPicker(_ app: XCUIApplication, asking question: String) {
        XCTAssertTrue(
            app.element("propose-pick-cabal-sheet").waitForExistence(timeout: 10),
            "S1.8: the cabal picker did not open as a sheet")
        let asked = app.element("propose-pick-cabal-question")
        XCTAssertTrue(asked.waitForExistence(timeout: 10), "S1.8: the cabal picker never showed its question")
        XCTAssertEqual(asked.label, question, "S1.8: the cabal picker does not ask '\(question)'")
        XCTAssertTrue(cabalRows(app).firstMatch.waitForExistence(timeout: 10), "S1.8: no cabal row in the picker")
        XCTAssertGreaterThanOrEqual(cabalRows(app).count, 2, "S1.8: the picker lists fewer than two cabals")
    }

    static func scrollToText(_ app: XCUIApplication, _ text: String, step: String) {
        scrollTo(app, app.staticTexts[text], named: "'\(text)'", step: step)
    }

    static func scrollTo(_ app: XCUIApplication, _ element: XCUIElement, named name: String, step: String) {
        var swipes = 0
        while !element.exists && swipes < 6 {
            app.element("asset-detail-root").swipeUp()
            swipes += 1
        }
        XCTAssertTrue(element.waitForExistence(timeout: 10), "\(step): no \(name) on the asset screen")
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
            openAsset(app, alpha, step: "S1.1")
            let title = app.navigationBars[alpha.name]
            XCTAssertTrue(title.exists, "S1.1: the asset screen is not titled '\(alpha.name)'")
        }

        recorder.step("S1.2", "read the hero") {
            XCTAssertEqual(app.staticTexts["asset-detail-name"].label, alpha.name, "S1.2: the name is not the seed's")
            let price = app.staticTexts["asset-detail-price"].label
            XCTAssertEqual(price, alpha.price, "S1.2: the price is not the seed's")
        }

        recorder.step("S1.3", "read the range chips") {
            XCTAssertTrue(
                waitForRangeChange(app, "Past day · \(alpha.ticker)", timeout: 15),
                "S1.3: the change chip reads '\(rangeChange(app))', not 'Past day · \(alpha.ticker)'")
            for range in ranges {
                XCTAssertTrue(rangeChip(app, range).waitForExistence(timeout: 5), "S1.3: no '\(range)' range chip")
            }
            XCTAssertTrue(rangeChip(app, "1D").isSelected, "S1.3: '1D' is not selected")
        }

        recorder.step("S1.4", "see the chart") {
            let chart = app.element("asset-detail-chart")
            XCTAssertTrue(chart.waitForExistence(timeout: 15), "S1.4: no chart for the seeded samples")
        }

        recorder.step("S1.5", "pick one week") {
            let week = rangeChip(app, "1W")
            week.tap()
            XCTAssertTrue(
                waitForRangeChange(app, "Past week · \(alpha.ticker)", timeout: 10),
                "S1.5: the change chip reads '\(rangeChange(app))', not 'Past week · \(alpha.ticker)'")
            XCTAssertTrue(week.isSelected, "S1.5: '1W' is not selected")
        }

        recorder.step("S1.6", "pick one year") {
            let year = rangeChip(app, "1Y")
            year.tap()
            XCTAssertTrue(
                waitForRangeChange(app, "Past year · \(alpha.ticker)", timeout: 10),
                "S1.6: the change chip reads '\(rangeChange(app))', not 'Past year · \(alpha.ticker)'")
            XCTAssertTrue(year.isSelected, "S1.6: '1Y' is not selected")
        }

        recorder.step("S1.7", "read Propose a buy") {
            let buy = app.buttons["asset-detail-propose-buy"]
            XCTAssertTrue(buy.waitForExistence(timeout: 15), "S1.7: no Propose a buy")
            app.scrollIntoReach(buy)
            XCTAssertEqual(buy.label, "Propose a buy", "S1.7: the CTA reads '\(buy.label)', not 'Propose a buy'")
            XCTAssertTrue(buy.isEnabled, "S1.7: Propose a buy is disabled for a tradable asset")
            XCTAssertTrue(app.staticTexts[buyCaption].exists, "S1.7: no '\(buyCaption)' under Propose a buy")
        }

        recorder.step("S1.8", "tap Propose a buy") {
            app.buttons["asset-detail-propose-buy"].tap()
            let question = "Which cabal should buy \(alpha.ticker)?"
            expectPicker(app, asking: question)
            XCTAssertEqual(
                app.element("propose-pick-cabal-sheet").label, "Choose a cabal",
                "S1.8: the cabal picker sheet is not titled 'Choose a cabal'")
        }

        recorder.step("S1.9", "pick a cabal and reach Amount") {
            reachAmount(app)
        }
    }

    static func reachAmount(_ app: XCUIApplication) {
        cabalRows(app).firstMatch.tap()
        XCTAssertTrue(
            app.element("propose-amount-screen").waitForExistence(timeout: 10),
            "S1.9: the cabal row did not open the Amount screen")
        XCTAssertTrue(
            app.navigationBars["Buy \(alpha.ticker)"].exists, "S1.9: the screen is not titled 'Buy \(alpha.ticker)'")
        let header = app.element("propose-amount-operation")
        for _ in 0..<10 where !header.exists {
            let retry = app.buttons["propose-amount-cash-error"]
            if retry.waitForExistence(timeout: 4) { retry.tap() }
        }
        XCTAssertTrue(header.waitForExistence(timeout: 5), "S1.9: no stock header on Amount")
        XCTAssertTrue(
            header.label.contains(alpha.name), "S1.9: the Amount header reads '\(header.label)', not \(alpha.name)")
    }

    static func stats(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open Journey Alpha") {
            openAsset(app, alpha, step: "S2.1")
        }

        recorder.step("S2.2", "scroll to Stats") {
            scrollTo(app, app.element("asset-stats"), named: "asset-stats", step: "S2.2")
        }

        recorder.step("S2.3", "see the 52-week bar") {
            let bar = app.element("asset-year-range")
            scrollTo(app, bar, named: "asset-year-range", step: "S2.3")
            XCTAssertTrue(
                bar.label.hasPrefix("52-week range"), "S2.3: the 52-week bar reads '\(bar.label)', not '52-week range'")
        }
    }

    static func preIpoBlock(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open Journey Private") {
            openAsset(app, preIpo, step: "S3.1")
            XCTAssertEqual(
                app.staticTexts["asset-detail-name"].label, preIpo.name, "S3.1: the name is not '\(preIpo.name)'")
        }

        recorder.step("S3.2", "see Also available from") {
            scrollToText(app, "Also available from", step: "S3.2")
            XCTAssertTrue(app.element(otherListingID).exists, "S3.2: no \(otherListingID) under 'Also available from'")
        }

        recorder.step("S3.3", "open the other listing") {
            let listing = app.element(otherListingID)
            app.scrollIntoReach(listing)
            listing.tap()
            XCTAssertTrue(
                app.element(sourceListingID).waitForExistence(timeout: 10),
                "S3.3: the listing did not open the asset screen that lists \(sourceListingID)")
            XCTAssertTrue(app.element("asset-detail-root").exists, "S3.3: no asset screen for the other listing")
            XCTAssertEqual(
                app.staticTexts["asset-detail-name"].firstMatch.label, preIpo.name,
                "S3.3: the name is not '\(preIpo.name)'")
        }
    }

    static func position(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S5.1", "open Journey Alpha") {
            openAsset(app, alpha, step: "S5.1")
        }

        recorder.step("S5.2", "see Cabals holding the stock") {
            scrollTo(app, app.element("asset-cabal-positions"), named: "asset-cabal-positions", step: "S5.2")
            let holding = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'asset-position-'")).firstMatch
            XCTAssertTrue(holding.exists, "S5.2: no cabal row under 'Cabals holding \(alpha.ticker)'")
        }

        recorder.step("S5.3", "tap Propose a sell") {
            let sell = app.buttons["asset-detail-propose-sell"]
            XCTAssertTrue(sell.waitForExistence(timeout: 5), "S5.3: no Propose a sell")
            app.scrollIntoReach(sell)
            sell.tap()
            XCTAssertTrue(
                app.navigationBars["Propose sell"].waitForExistence(timeout: 10),
                "S5.3: Propose sell did not open the propose screen")
        }
    }

    static func zoomAndPan(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S4.1", "open Journey Alpha") {
            openAsset(app, alpha, step: "S4.1")
            XCTAssertTrue(
                app.element("asset-detail-chart").waitForExistence(timeout: 15), "S4.1: no chart for the seeded samples"
            )
        }

        recorder.step("S4.2", "pinch the chart") {
            let chart = app.element("asset-detail-chart")
            chart.pinch(withScale: 2, velocity: 1)
            XCTAssertTrue(chart.exists, "S4.2: the chart is gone after the pinch")
        }

        recorder.step("S4.3", "pan back in time") {
            app.element("asset-detail-chart").swipeRight()
            XCTAssertTrue(
                app.buttons["asset-chart-jump"].waitForExistence(timeout: 10),
                "S4.3: no jump-to-latest button after panning back")
        }

        recorder.step("S4.4", "jump to the latest bar") {
            let jump = app.buttons["asset-chart-jump"]
            jump.tap()
            let gone = NSPredicate(format: "exists == false")
            let wait = XCTNSPredicateExpectation(predicate: gone, object: jump)
            XCTAssertEqual(XCTWaiter().wait(for: [wait], timeout: 10), .completed, "S4.4: the jump button stayed")
        }
    }
}
