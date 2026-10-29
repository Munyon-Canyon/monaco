import XCTest

nonisolated final class AssetDetailSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String, textSize: String? = nil) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoAssetDetailSample", scenario]
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
            app.navigationBars.firstMatch.waitForExistence(timeout: 30), "\(context) never drew the stock screen")
    }

    @MainActor
    private func scrollTo(_ app: XCUIApplication, _ element: XCUIElement, _ context: String) {
        for _ in 0..<10 where !(element.exists && element.isHittable) {
            app.swipeUp()
        }
        XCTAssertTrue(element.exists, "\(context) never scrolled into view")
    }

    @MainActor
    func testEveryScenarioDraws() throws {
        let scenarios = [
            "open", "loss", "noHolders", "noProposals", "preIpo", "untradable", "paused", "stale", "emptyChart",
            "chartFailed", "loading", "failed",
        ]
        for scenario in scenarios {
            let app = launch(scenario)
            waitForScreen(app, scenario)
            attachScreenshot(app, name: "stock-\(scenario)")
            app.terminate()
        }
    }

    @MainActor
    func testOpenShowsTheHeaderPriceChartAndActions() throws {
        let app = launch("open")
        waitForScreen(app, "open")

        XCTAssertTrue(anyElement(app, "asset-detail-name").waitForExistence(timeout: 30), "no name")
        XCTAssertEqual(anyElement(app, "asset-detail-name").label, "Alphabet")
        XCTAssertTrue(anyElement(app, "asset-detail-price").exists, "no price")
        XCTAssertTrue(anyElement(app, "asset-chart-ranges").waitForExistence(timeout: 30), "no range control")
        XCTAssertTrue(app.buttons["asset-chart-style-candles"].isSelected, "the chart should open on candles")
        let star = app.buttons["asset-watch-star"]
        XCTAssertEqual(star.label, "Remove from watchlist", "the open scenario watches this stock")
        XCTAssertTrue(star.isSelected)
        attachScreenshot(app, name: "stock-open-top")
        let buy = app.buttons["asset-detail-propose-buy"]
        scrollTo(app, buy, "Propose a buy")
        XCTAssertEqual(buy.label, "Propose a buy")
        XCTAssertTrue(buy.isEnabled)
        XCTAssertTrue(app.staticTexts["Your cabal votes before anything is bought or sold"].exists)
        let sell = app.buttons["asset-detail-propose-sell"]
        XCTAssertTrue(sell.exists, "a voting cabal holds this stock")
        XCTAssertEqual(buy.frame.minY, sell.frame.minY, accuracy: 1, "buy and sell should share one row")
        XCTAssertEqual(buy.frame.width, sell.frame.width, accuracy: 1, "buy and sell should be equal widths")
        XCTAssertLessThan(buy.frame.maxX, sell.frame.minX, "buy sits left of sell")
    }

    @MainActor
    func testTheStarAddsAndRemovesTheStock() throws {
        let app = launch("noProposals")
        waitForScreen(app, "noProposals")
        let star = app.buttons["asset-watch-star"]
        XCTAssertTrue(star.waitForExistence(timeout: 30), "no star")
        XCTAssertEqual(star.label, "Add to watchlist")
        XCTAssertFalse(star.isSelected)

        star.tap()
        XCTAssertEqual(star.label, "Remove from watchlist")
        XCTAssertTrue(star.isSelected)
        attachScreenshot(app, name: "stock-starred")

        star.tap()
        XCTAssertEqual(star.label, "Add to watchlist")
        XCTAssertFalse(star.isSelected)
    }

    @MainActor
    func testTheChartSwitchesBetweenCandlesAndLine() throws {
        let app = launch("open")
        waitForScreen(app, "open")
        let line = app.buttons["asset-chart-style-line"]
        XCTAssertTrue(line.waitForExistence(timeout: 30), "no line toggle")
        addTeardownBlock { @MainActor in app.buttons["asset-chart-style-candles"].tap() }

        line.tap()

        XCTAssertTrue(line.isSelected, "the line toggle should read as selected")
        XCTAssertFalse(app.buttons["asset-chart-style-candles"].isSelected)
        attachScreenshot(app, name: "stock-line")
    }

    @MainActor
    func testAnUntradableStockShowsTheNoticeAndADisabledButton() throws {
        let app = launch("untradable")
        waitForScreen(app, "untradable")

        XCTAssertTrue(
            app.staticTexts["This stock is unavailable for new proposals."].waitForExistence(timeout: 30),
            "no unavailable notice")
        let buy = app.buttons["asset-detail-propose-buy"]
        scrollTo(app, buy, "the buy button")
        XCTAssertEqual(buy.label, "Unavailable")
        XCTAssertFalse(buy.isEnabled)
    }

    @MainActor
    func testAPausedStockCannotBeProposed() throws {
        let app = launch("paused")
        waitForScreen(app, "paused")

        XCTAssertTrue(
            app.staticTexts["Trading paused right now."].waitForExistence(timeout: 30), "no paused notice")
        let buy = app.buttons["asset-detail-propose-buy"]
        scrollTo(app, buy, "the buy button")
        XCTAssertEqual(buy.label, "Propose a buy")
        XCTAssertFalse(buy.isEnabled)
    }

    @MainActor
    func testNoHoldersAndNoProposalsSayTheirPiece() throws {
        let holders = launch("noHolders")
        waitForScreen(holders, "noHolders")
        scrollTo(holders, holders.staticTexts["No visible holders"], "the empty holders state")
        XCTAssertTrue(holders.staticTexts["Private holdings are not shown."].exists)
        attachScreenshot(holders, name: "stock-no-holders")
        holders.terminate()

        let proposals = launch("noProposals")
        waitForScreen(proposals, "noProposals")
        scrollTo(proposals, proposals.staticTexts["No visible proposals for this asset."], "the quiet empty line")
        XCTAssertTrue(proposals.staticTexts["Proposals by stock open soon."].exists)
        attachScreenshot(proposals, name: "stock-no-proposals")
    }

    @MainActor
    func testTheStatsSayWhatOpensSoon() throws {
        let app = launch("open")
        waitForScreen(app, "open")

        scrollTo(app, app.staticTexts["Market stats"], "Market stats")
        scrollTo(app, app.staticTexts["Market cap and supply open soon."], "the opens-soon line")
        XCTAssertTrue(app.staticTexts["Company profile opens soon."].exists || anyElement(app, "asset-about").exists)
        attachScreenshot(app, name: "stock-stats")
    }

    @MainActor
    func testAFailedLoadIsTheFullScreenErrorWithRetry() throws {
        let app = launch("failed")
        waitForScreen(app, "failed")

        XCTAssertTrue(app.staticTexts["Couldn't load this view"].waitForExistence(timeout: 30), "no failure state")
        XCTAssertEqual(app.buttons["asset-detail-failed"].label, "Try again")
    }

    @MainActor
    func testTheScreenSurvivesAccessibilityTextSizes() throws {
        let app = launch("open", textSize: "UICTContentSizeCategoryAccessibilityXXXL")
        waitForScreen(app, "open at AX5")

        XCTAssertTrue(anyElement(app, "asset-detail-name").waitForExistence(timeout: 30), "no name at AX5")
        XCTAssertTrue(app.buttons["asset-watch-star"].exists, "the star should survive AX5")
        attachScreenshot(app, name: "stock-ax5")
    }
}
