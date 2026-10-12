import XCTest

nonisolated final class PnlChartSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ flag: String, _ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = [flag, scenario, "-MonacoScrubHolds"]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func attach(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func bringIntoView(_ app: XCUIApplication, _ chart: XCUIElement) {
        var swipes = 0
        while !chart.isHittable && swipes < 8 {
            app.swipeUp()
            swipes += 1
        }
    }

    @MainActor
    private func exercise(_ app: XCUIApplication, chartID: String, prefix: String) {
        let chart = element(app, chartID)
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no \(chartID)")
        bringIntoView(app, chart)
        _ = XCTWaiter().wait(for: [XCTestExpectation()], timeout: 1.5)
        attach(app, "\(prefix)-chart")

        let start = chart.coordinate(withNormalizedOffset: CGVector(dx: 0.55, dy: 0.5))
        let end = chart.coordinate(withNormalizedOffset: CGVector(dx: 0.45, dy: 0.5))
        start.press(forDuration: 0.7, thenDragTo: end)
        XCTAssertTrue(element(app, "asset-chart-crosshair").waitForExistence(timeout: 5), "\(prefix): no crosshair")
        attach(app, "\(prefix)-held")
    }

    @MainActor
    func testHomePortfolioChartHoldsAndPinches() throws {
        let app = launch("-MonacoHomeSample", "populated")
        exercise(app, chartID: "home-pnl-chart", prefix: "pnl-home")
        let chart = element(app, "home-pnl-chart")
        app.terminate()
        let relaunched = launch("-MonacoHomeSample", "populated")
        let again = element(relaunched, "home-pnl-chart")
        XCTAssertTrue(again.waitForExistence(timeout: 30), "no chart after relaunch")
        again.pinch(withScale: 2.5, velocity: 2)
        XCTAssertTrue(again.exists)
        XCTAssertTrue(
            element(relaunched, "asset-chart-jump").waitForExistence(timeout: 5), "no jump button after zooming in")
        attach(relaunched, "pnl-home-pinched")
        XCTAssertNotNil(chart)
    }

    @MainActor
    func testHomePageStillScrollsWhenSwipingOnTheChart() throws {
        let app = launch("-MonacoHomeSample", "populated")
        let chart = element(app, "home-pnl-chart")
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no chart")
        let before = chart.frame.minY

        chart.swipeUp()

        XCTAssertLessThan(chart.frame.minY, before - 20, "a vertical swipe on the chart did not scroll the page")
    }

    @MainActor
    func testCabalValueChartHoldsAndPinches() throws {
        let app = launch("-MonacoGroupDetailSample", "populated")
        exercise(app, chartID: "cabal-value-chart", prefix: "pnl-cabal")
        app.terminate()
        let relaunched = launch("-MonacoGroupDetailSample", "populated")
        let chart = element(relaunched, "cabal-value-chart")
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no cabal chart after relaunch")
        bringIntoView(relaunched, chart)
        chart.pinch(withScale: 2.5, velocity: 2)
        XCTAssertTrue(chart.exists)
        attach(relaunched, "pnl-cabal-pinched")
    }
}
