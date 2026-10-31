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
        XCTAssertEqual(anyElement(app, "asset-detail-name").label, "NVIDIA")
        XCTAssertTrue(anyElement(app, "asset-detail-price").exists, "no price")
        XCTAssertTrue(app.buttons["asset-share"].exists, "no share button")
        XCTAssertEqual(app.buttons["asset-share"].label, "Share NVIDIA")
        XCTAssertTrue(anyElement(app, "asset-chart-ranges").waitForExistence(timeout: 30), "no range control")
        let star = app.buttons["asset-watch-star"]
        XCTAssertEqual(star.label, "Remove from watchlist", "the open scenario watches this stock")
        XCTAssertTrue(star.isSelected)
        attachScreenshot(app, name: "stock-open-top")
        let buy = app.buttons["asset-detail-propose-buy"]
        scrollTo(app, buy, "Propose buy")
        XCTAssertEqual(buy.label, "Propose buy")
        XCTAssertTrue(buy.isEnabled)
        XCTAssertFalse(app.buttons["asset-detail-propose-sell"].exists, "no cabal holds this stock")
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
        XCTAssertEqual(buy.label, "Propose buy")
        XCTAssertFalse(buy.isEnabled)
    }

    @MainActor
    func testNoHoldersAndNoProposalsLeaveOutInYourCabals() throws {
        let holders = launch("noHolders")
        waitForScreen(holders, "noHolders")
        let buy = holders.buttons["asset-detail-propose-buy"]
        scrollTo(holders, buy, "the buy button")
        XCTAssertFalse(holders.buttons["asset-detail-propose-sell"].exists, "no cabal holds this stock")
        XCTAssertFalse(anyElement(holders, "asset-in-your-cabals").exists, "no cabals section without holders")
        attachScreenshot(holders, name: "stock-no-holders")
        holders.terminate()

        let proposals = launch("noProposals")
        waitForScreen(proposals, "noProposals")
        scrollTo(proposals, proposals.buttons["asset-detail-propose-buy"], "the buy button")
        let openProposal = proposals.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'asset-open-proposal-'")
        ).firstMatch
        XCTAssertFalse(openProposal.exists, "no open proposal for this stock")
        attachScreenshot(proposals, name: "stock-no-proposals")
    }

    @MainActor
    func testAHeldPreIpoStockListsItsCabalsAndProposals() throws {
        let app = launch("preIpo")
        waitForScreen(app, "preIpo")
        scrollTo(app, anyElement(app, "asset-in-your-cabals"), "In your cabals")
        XCTAssertTrue(app.buttons["asset-detail-propose-sell"].exists, "a cabal holds this stock")
        attachScreenshot(app, name: "stock-pre-ipo-in-your-cabals")
    }

    @MainActor
    func testNoOpensSoonCopyAppearsOnTheStockPage() throws {
        for scenario in ["open", "preIpo"] {
            let app = launch(scenario)
            waitForScreen(app, scenario)
            let opensSoon = app.descendants(matching: .any).matching(
                NSPredicate(format: "label CONTAINS[c] 'open soon' OR label CONTAINS[c] 'opens soon'")
            ).firstMatch
            for _ in 0..<10 {
                XCTAssertFalse(opensSoon.exists, "\(scenario) shows opens-soon copy: \(opensSoon.label)")
                app.swipeUp()
            }
            XCTAssertFalse(opensSoon.exists, "\(scenario) shows opens-soon copy: \(opensSoon.label)")
            attachScreenshot(app, name: "stock-\(scenario)-bottom")
            app.terminate()
        }
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
