import XCTest

private struct FrameReport: Decodable {
    let frames: Int
    let dropped: Int
    let hitches: Int
    let worstMilliseconds: Double

    private enum CodingKeys: String, CodingKey {
        case frames, dropped, hitches
        case worstMilliseconds = "worst_ms"
    }
}

nonisolated final class AssetChartSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(holdsSelection: Bool = false, scenario: String = "open") -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoAssetDetailSample", scenario]
        if holdsSelection { app.launchArguments.append("-MonacoScrubHolds") }
        app.launch()
        return app
    }

    @MainActor
    private func attach(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testPinchPanAndJumpToLatest() throws {
        let app = launch()
        let chart = element(app, "asset-detail-chart")
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no chart")
        XCTAssertFalse(app.buttons["asset-chart-jump"].exists, "the jump button shows at the latest bar")
        attach(app, "chart-initial")

        chart.pinch(withScale: 2.5, velocity: 2)
        XCTAssertTrue(chart.exists)
        attach(app, "chart-pinched-in")

        chart.swipeRight()
        XCTAssertTrue(app.buttons["asset-chart-jump"].waitForExistence(timeout: 10), "no jump button after panning")
        attach(app, "chart-panned")

        app.buttons["asset-chart-jump"].tap()
        let gone = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "exists == false"), object: app.buttons["asset-chart-jump"])
        XCTAssertEqual(XCTWaiter().wait(for: [gone], timeout: 10), .completed, "the jump button stayed")
        attach(app, "chart-back-at-latest")

        chart.pinch(withScale: 0.4, velocity: -2)
        attach(app, "chart-pinched-out")
    }

    @MainActor
    func testAVerticalSwipeOnTheChartScrollsThePage() throws {
        let app = launch()
        let chart = element(app, "asset-detail-chart")
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no chart")
        let before = chart.frame.minY

        chart.swipeUp()

        XCTAssertLessThan(chart.frame.minY, before - 20, "the page did not scroll under a vertical swipe on the chart")
        attach(app, "chart-page-scrolled")
    }

    @MainActor
    func testHoldingShowsTheCrosshairAndTheHeroFollows() throws {
        let app = launch(holdsSelection: true)
        let chart = element(app, "asset-detail-chart")
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no chart")
        let price = app.staticTexts["asset-detail-price"]
        let before = price.label

        let start = chart.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
        let end = chart.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.5))
        start.press(forDuration: 0.6, thenDragTo: end)

        XCTAssertTrue(element(app, "asset-chart-crosshair").waitForExistence(timeout: 5), "no crosshair")
        XCTAssertNotEqual(price.label, before, "the hero price did not follow the crosshair")
        attach(app, "chart-crosshair")
    }

    @MainActor
    func testUnknownScenarioFailsLoudly() throws {
        let app = launch(scenario: "noSuchScenario")
        XCTAssertTrue(
            element(app, "sample-unknown-scenario").waitForExistence(timeout: 30), "no unknown-scenario screen")
    }

    @MainActor
    func testPanHitchesWithThreeHundredBars() throws {
        try measurePan(scenario: "open")
    }

    @MainActor
    func testPanHitchesWithTwentyTwoHundredBars() throws {
        try measurePan(scenario: "candlesLong")
    }

    @MainActor
    private func frameReport(_ app: XCUIApplication, newerThan frames: Int) throws -> FrameReport {
        let report = app.otherElements["frame-stats-report"]
        XCTAssertTrue(report.waitForExistence(timeout: 30), "no frame-stats-report")
        var latest = try decode(report)
        let deadline = Date().addingTimeInterval(20)
        while latest.frames <= frames && Date() < deadline {
            _ = XCTWaiter().wait(for: [XCTestExpectation()], timeout: 1)
            latest = try decode(report)
        }
        return latest
    }

    @MainActor
    private func decode(_ report: XCUIElement) throws -> FrameReport {
        let json = try XCTUnwrap(report.value as? String, "frame-stats-report has no value")
        return try JSONDecoder().decode(FrameReport.self, from: Data(json.utf8))
    }

    @MainActor
    func testPageScrollBaseline() throws {
        try measurePan(scenario: "open", baseline: true)
    }

    @MainActor
    private func measurePan(scenario: String, baseline: Bool = false) throws {
        let app = launch(scenario: scenario)
        app.launchArguments.append("-MonacoFrameStats")
        app.terminate()
        app.launch()
        let chart = element(app, "asset-detail-chart")
        XCTAssertTrue(chart.waitForExistence(timeout: 30), "no chart")
        let before = try frameReport(app, newerThan: 0)
        let name = app.staticTexts["asset-detail-name"]
        for _ in 0..<3 {
            if baseline {
                name.swipeUp(velocity: .fast)
                name.swipeUp(velocity: .fast)
                name.swipeDown(velocity: .fast)
                name.swipeDown(velocity: .fast)
            } else {
                chart.swipeRight(velocity: .fast)
                chart.swipeRight(velocity: .fast)
                chart.swipeLeft(velocity: .fast)
                chart.swipeLeft(velocity: .fast)
            }
        }
        let after = try frameReport(app, newerThan: before.frames)
        let summary =
            "\(scenario)\(baseline ? " page-scroll baseline" : " pan"): frames \(after.frames - before.frames), dropped \(after.dropped - before.dropped), "
            + "hitches \(after.hitches - before.hitches), worst \(after.worstMilliseconds) ms"
        print("PERF " + summary)
        add(XCTAttachment(string: summary))
    }

    @MainActor
    func testScenarioScreenshots() throws {
        let shots: [(name: String, scenario: String)] = [
            ("asset-open", "open"),
            ("asset-candles-long", "candlesLong"),
            ("asset-empty-chart", "emptyChart"),
            ("asset-chart-failed", "chartFailed"),
            ("asset-candles-sparse", "candlesSparse"),
        ]
        for shot in shots {
            let app = XCUIApplication()
            app.launchArguments = ["-MonacoAssetDetailSample", shot.scenario]
            app.launch()
            XCTAssertTrue(app.staticTexts["asset-detail-name"].waitForExistence(timeout: 30), shot.name)
            _ = XCTWaiter().wait(for: [XCTestExpectation()], timeout: 2)
            attach(app, shot.name)
            app.terminate()
        }
    }

    @MainActor
    func testRangeChipsReloadTheChartAndTheChange() throws {
        let app = launch()
        XCTAssertTrue(element(app, "asset-detail-chart").waitForExistence(timeout: 30), "no chart")
        for (chip, caption) in [
            ("1W", "Past week"), ("1M", "Past month"), ("6M", "Past six months"), ("1D", "Past day"),
        ] {
            app.buttons["asset-range-\(chip)"].tap()
            let change = app.staticTexts.matching(identifier: "asset-detail-range-change")
                .matching(NSPredicate(format: "label CONTAINS %@", caption)).firstMatch
            _ = change.waitForExistence(timeout: 10)
            attach(app, "chip-\(chip)")
            XCTAssertTrue(change.exists, "\(chip): the change chip never read '\(caption)'")
            XCTAssertTrue(element(app, "asset-detail-chart").waitForExistence(timeout: 10), "\(chip): no chart")
        }
        attach(app, "range-chips")
    }
}
