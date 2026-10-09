//
//  StocksTabSampleUITests.swift
//  MonacoUITests
//
//  QA coverage for the live Stocks tab against the debug sample harness
//  (-MonacoStocksTabSample full|loading|failed|empty). No sign-in and no backend:
//  the harness answers the asset reads from sample data (Alphabet, T-SpaceX and
//  Newco), so every state the tab can be in is screenshottable here.
//
//  Everything is asked of an element that really publishes itself: the navigation
//  bar, the search field, a chip's button, a row's button. The identifiers on the
//  SwiftUI containers around them are for reading the hierarchy, not for finding
//  it — a VStack does not reliably reach the accessibility tree, and a test that
//  waits on one is a test that lies.
//

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

    /// "The tab drew" is asked of the navigation bar: it is there in every state,
    /// including the skeleton and both failures.
    @MainActor
    private func waitForTab(_ app: XCUIApplication, _ context: String) {
        XCTAssertTrue(
            app.navigationBars["Stocks"].waitForExistence(timeout: 30),
            "\(context) never drew the Stocks tab"
        )
    }

    @MainActor
    private func waitForSearchField(_ app: XCUIApplication, _ context: String) -> XCUIElement {
        let field = app.textFields["monaco-search-field"].firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 25), "\(context) never drew the search field")
        return field
    }

    @MainActor
    private func assertChips(_ app: XCUIApplication, _ context: String) {
        for chip in ["All", "Popular", "Pre-IPO"] {
            XCTAssertTrue(
                app.buttons["stocks-chip-\(chip)"].waitForExistence(timeout: 15),
                "\(context) is missing the \(chip) chip"
            )
        }
    }

    /// Every scenario reaches a drawn tab. This is the screenshot sweep: one
    /// attachment per state the backend can put the tab in.
    @MainActor
    func testEveryScenarioDraws() throws {
        for scenario in ["full", "loading", "failed", "empty"] {
            let app = launch(scenario)
            waitForTab(app, scenario)
            attachScreenshot(app, name: "stocks-\(scenario)")
            app.terminate()
        }
    }

    /// The populated tab shows the search field, all three chips and a row for
    /// each sample stock.
    @MainActor
    func testPopulatedTabShowsSearchChipsAndRows() throws {
        let app = launch("full")
        waitForTab(app, "full")

        _ = waitForSearchField(app, "full")
        assertChips(app, "full")
        XCTAssertTrue(anyElement(app, "assets-row-GOOGLx").waitForExistence(timeout: 25), "the Alphabet row is missing")
        XCTAssertTrue(anyElement(app, "assets-row-tSpaceX").exists, "the SpaceX row is missing")
        XCTAssertTrue(anyElement(app, "assets-row-NEWCO").exists, "the Newco row is missing")
        attachScreenshot(app, name: "stocks-rows")
    }

    /// A catalogue with nothing in it gets an answer, not a failure.
    @MainActor
    func testEmptyCatalogueIsAnEmptyStateNotAnError() throws {
        let app = launch("empty")
        waitForTab(app, "empty")

        XCTAssertTrue(app.staticTexts["No stocks to show"].waitForExistence(timeout: 25), "the empty state never drew")
        XCTAssertFalse(app.staticTexts["Couldn't load stocks."].exists, "an empty catalogue is not the failure state")
        XCTAssertFalse(anyElement(app, "assets-row-GOOGLx").exists, "no rows should draw for an empty catalogue")
        XCTAssertTrue(app.textFields["monaco-search-field"].exists, "the search field should stay live")
        assertChips(app, "empty")
    }

    /// A failed read leaves the search field and chips in place around it.
    @MainActor
    func testAFailedReadLeavesTheSearchAndChipsAlone() throws {
        let app = launch("failed")
        waitForTab(app, "failed")

        XCTAssertTrue(app.staticTexts["Couldn't load stocks."].waitForExistence(timeout: 25), "the failure never drew")
        XCTAssertTrue(app.textFields["monaco-search-field"].exists, "the search field should stay live")
        assertChips(app, "failed")
        XCTAssertFalse(anyElement(app, "assets-row-GOOGLx").exists, "a failed read must not show rows")
        attachScreenshot(app, name: "stocks-failed")
    }

    /// A catalogue that will not load gets the retry, not a half-empty tab.
    @MainActor
    func testAFailedCatalogueOffersRetry() throws {
        let app = launch("failed")
        waitForTab(app, "failed")

        XCTAssertTrue(app.staticTexts["Couldn't load stocks."].waitForExistence(timeout: 25), "the failure never drew")
        let retry = app.buttons["assets-retry"].firstMatch
        XCTAssertTrue(retry.exists, "the failure should offer Try again")
        XCTAssertEqual(retry.label, "Try again", "the retry button should say what it does")
    }

    /// A stock with no price and no change still renders as a row. The old list
    /// would have been happy to draw a flat line here, which reads as "it did not
    /// move". Newco is the sample with neither.
    @MainActor
    func testAStockWithoutAPriceStillRenders() throws {
        let app = launch("full")
        waitForTab(app, "full")

        XCTAssertTrue(
            anyElement(app, "assets-row-NEWCO").waitForExistence(timeout: 25),
            "the unpriced Newco row should still draw"
        )
        attachScreenshot(app, name: "stocks-no-price")
    }

    /// Searching replaces the list; clearing the field brings it back.
    @MainActor
    func testSearchReplacesTheListAndComesBack() throws {
        let app = launch("full")
        waitForTab(app, "full")
        XCTAssertTrue(anyElement(app, "assets-row-NEWCO").waitForExistence(timeout: 25), "the list never drew")

        let field = waitForSearchField(app, "full")
        field.tap()
        field.typeText("Alphabet")

        XCTAssertTrue(anyElement(app, "assets-row-GOOGLx").waitForExistence(timeout: 25), "search results never drew")
        XCTAssertTrue(
            waitForAbsence(of: anyElement(app, "assets-row-NEWCO")),
            "a search is a different question: Newco does not match Alphabet"
        )

        app.buttons["Clear search"].firstMatch.tap()
        XCTAssertTrue(anyElement(app, "assets-row-NEWCO").waitForExistence(timeout: 25), "browsing never came back")
        XCTAssertTrue(anyElement(app, "assets-row-tSpaceX").exists, "the full list should be back")
    }

    @MainActor
    private func waitForAbsence(of element: XCUIElement, timeout: TimeInterval = 25) -> Bool {
        let gone = expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: element)
        return XCTWaiter().wait(for: [gone], timeout: timeout) == .completed
    }

    /// Each chip narrows the list to its own rows, and the selected chip says so.
    /// This is the tab's filter: Popular keeps Alphabet, Pre-IPO keeps SpaceX.
    @MainActor
    func testAChipNarrowsTheListToItsOwnRows() throws {
        let app = launch("full")
        waitForTab(app, "full")
        XCTAssertTrue(anyElement(app, "assets-row-NEWCO").waitForExistence(timeout: 25), "the list never drew")

        app.buttons["stocks-chip-Popular"].tap()
        XCTAssertTrue(anyElement(app, "assets-row-GOOGLx").waitForExistence(timeout: 25), "Popular lost its row")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "assets-row-tSpaceX")), "Popular should not list SpaceX")
        XCTAssertTrue(app.buttons["stocks-chip-Popular"].isSelected, "the Popular chip should read as selected")

        app.buttons["stocks-chip-Pre-IPO"].tap()
        XCTAssertTrue(anyElement(app, "assets-row-tSpaceX").waitForExistence(timeout: 25), "Pre-IPO lost its row")
        XCTAssertTrue(waitForAbsence(of: anyElement(app, "assets-row-GOOGLx")), "Pre-IPO should not list Alphabet")
        XCTAssertTrue(app.buttons["stocks-chip-Pre-IPO"].isSelected, "the Pre-IPO chip should read as selected")
        attachScreenshot(app, name: "stocks-chip-pre-ipo")
    }

    /// The rows stack at accessibility text sizes rather than squeezing the name
    /// out, and the tab still draws.
    @MainActor
    func testTheTabSurvivesAccessibilityTextSizes() throws {
        let app = launch("full", textSize: "UICTContentSizeCategoryAccessibilityXXXL")
        waitForTab(app, "full at AX5")

        XCTAssertTrue(
            anyElement(app, "assets-row-GOOGLx").waitForExistence(timeout: 30),
            "the first row should draw at accessibility text sizes"
        )
        attachScreenshot(app, name: "stocks-ax5")
    }

    /// The search field and chips are the tab's scanning controls. At AX5 they
    /// must still be reachable and the tab must not grow a section it never had:
    /// the old mover strip stepped aside here, and nothing like it comes back.
    @MainActor
    func testTheChipsStayReachableAtAccessibilityTextSizes() throws {
        let normal = launch("full")
        waitForTab(normal, "full")
        assertChips(normal, "full")
        XCTAssertTrue(anyElement(normal, "assets-row-GOOGLx").waitForExistence(timeout: 25), "a row should be built")
        XCTAssertFalse(normal.staticTexts["Top movers"].exists, "the tab has no mover strip")
        attachScreenshot(normal, name: "stocks-chips-default")
        normal.terminate()

        let large = launch("full", textSize: "UICTContentSizeCategoryAccessibilityXXXL")
        waitForTab(large, "full at AX5")
        XCTAssertTrue(anyElement(large, "assets-row-GOOGLx").waitForExistence(timeout: 30), "no row at AX5")
        XCTAssertTrue(large.textFields["monaco-search-field"].exists, "the search field should survive AX5")
        XCTAssertTrue(large.buttons["stocks-chip-All"].exists, "the chips should survive AX5")
        XCTAssertFalse(large.staticTexts["Top movers"].exists, "the tab has no mover strip at AX5 either")
        attachScreenshot(large, name: "stocks-chips-ax5")
    }

    /// The day-change pill is its own control at the trailing edge of a row whose
    /// job is to navigate. Growing it must not grow what it swallows: the row's own
    /// hit point is still the row's.
    @MainActor
    func testTappingTheMiddleOfARowOpensTheStock() throws {
        let app = launch("full")
        waitForTab(app, "full")

        let row = anyElement(app, "assets-row-GOOGLx")
        XCTAssertTrue(row.waitForExistence(timeout: 25), "the Alphabet row is missing")
        row.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()

        let leftTheTab = NSPredicate(format: "exists == false")
        expectation(for: leftTheTab, evaluatedWith: app.navigationBars["Stocks"])
        waitForExpectations(timeout: 25)
    }

    /// Tapping a row opens the stock it is about. The pushed screen is the live
    /// detail view, so what is asserted is that the tab was left at all: its
    /// navigation bar is gone once it has been.
    @MainActor
    func testTappingARowOpensTheStock() throws {
        let app = launch("full")
        waitForTab(app, "full")

        let row = anyElement(app, "assets-row-GOOGLx")
        XCTAssertTrue(row.waitForExistence(timeout: 25), "the Alphabet row is missing")
        row.coordinate(withNormalizedOffset: CGVector(dx: 0.25, dy: 0.5)).tap()

        let leftTheTab = NSPredicate(format: "exists == false")
        expectation(for: leftTheTab, evaluatedWith: app.navigationBars["Stocks"])
        waitForExpectations(timeout: 25)
    }
}
