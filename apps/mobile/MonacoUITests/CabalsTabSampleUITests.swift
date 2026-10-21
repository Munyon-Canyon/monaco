import XCTest

nonisolated final class CabalsTabSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launchApp(scenario: String? = nil) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCabalsTabSample"]
        if let scenario {
            app.launchArguments += ["-MonacoCabalsTabSampleScenario", scenario]
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
    private func isOnGlass(_ app: XCUIApplication, _ element: XCUIElement) -> Bool {
        guard element.exists else { return false }
        let frame = element.frame
        guard frame.width > 1, frame.height > 1, !frame.isNull, !frame.isInfinite else { return false }
        return app.frame.contains(CGPoint(x: frame.midX, y: frame.midY))
    }

    @MainActor
    @discardableResult
    private func scrollUntilHittable(_ app: XCUIApplication, _ element: XCUIElement, attempts: Int = 8) -> Bool {
        for _ in 0..<attempts {
            if isOnGlass(app, element), element.isHittable { return true }
            app.swipeUp()
        }
        return isOnGlass(app, element) && element.isHittable
    }

    private let qaPotID = "01890a5d-ac96-774b-bcce-b302099a8060"

    @MainActor
    @discardableResult
    private func reveal(_ app: XCUIApplication, _ identifier: String, attempts: Int = 6) -> XCUIElement {
        let element = anyElement(app, identifier)
        for _ in 0..<attempts {
            if element.exists, isOnGlass(app, element) { return element }
            app.swipeUp()
        }
        return element
    }

    @MainActor
    func testOverviewShowsYourCabalsAndBoard() throws {
        let app = launchApp()

        let root = anyElement(app, "cabals-root")
        XCTAssertTrue(root.waitForExistence(timeout: 10), "cabals-root should exist after launch")

        XCTAssertTrue(anyElement(app, "cabals-list").waitForExistence(timeout: 10), "your cabals list should exist")
        XCTAssertTrue(
            anyElement(app, "cabals-list-card-\(qaPotID)").waitForExistence(timeout: 5),
            "the QA pot card should exist in your cabals"
        )
        XCTAssertFalse(anyElement(app, "cabals-list-new").exists, "the dashed New cabal card is gone")
        XCTAssertTrue(anyElement(app, "cabals-filter-field").exists, "the tab has a search field")

        XCTAssertTrue(reveal(app, "cabals-board").waitForExistence(timeout: 10), "the board should exist")

        attachScreenshot(app, name: "01-overview")
    }

    @MainActor
    func testBoardShowsRankedRows() throws {
        let app = launchApp()

        let board = reveal(app, "cabals-board")
        XCTAssertTrue(board.waitForExistence(timeout: 10), "the board should exist")
        XCTAssertTrue(reveal(app, "cabals-board-list").waitForExistence(timeout: 10), "the board list should load")

        for rank in 1...3 {
            XCTAssertTrue(
                reveal(app, "cabals-board-row-user-\(rank)").waitForExistence(timeout: 5),
                "board row \(rank) should be visible after scrolling"
            )
        }
        XCTAssertFalse(anyElement(app, "cabals-board-empty").exists, "a ranked board is not empty")

        attachScreenshot(app, name: "02-board")
    }

    @MainActor
    func testUnloadedCabalsDoNotClaimTheMemberHasNone() throws {
        let app = launchApp(scenario: "cabalsUnavailable")

        let root = anyElement(app, "cabals-root")
        XCTAssertTrue(root.waitForExistence(timeout: 10), "cabals-root should exist after launch")

        XCTAssertTrue(
            app.buttons["cabals-list-retry"].waitForExistence(timeout: 10),
            "an unloaded cabals list should say it failed and offer a retry"
        )
        XCTAssertTrue(app.staticTexts["Couldn't load this view"].exists, "the failure is spelled out")
        XCTAssertFalse(
            anyElement(app, "cabals-list-empty").exists,
            "an unloaded cabals list must not claim 'No cabals yet'"
        )

        XCTAssertTrue(
            reveal(app, "cabals-board-list-error").waitForExistence(timeout: 10),
            "an unloaded board should offer a retry"
        )
        XCTAssertFalse(anyElement(app, "cabals-board-empty").exists, "an unloaded board must not claim to be empty")

        attachScreenshot(app, name: "07-cabals-unavailable")
    }

    @MainActor
    func testBoardRowAnnouncesItsRank() throws {
        let app = launchApp()

        XCTAssertTrue(reveal(app, "cabals-board").waitForExistence(timeout: 10), "the board should exist")

        let topRow = reveal(app, "cabals-board-row-user-1")
        XCTAssertTrue(topRow.waitForExistence(timeout: 10), "the top board row should exist")
        XCTAssertTrue(
            topRow.label.hasPrefix("First,"),
            "the rank is the point of this board; rank 1 is spoken as First, got: \(topRow.label)"
        )

        let secondRow = reveal(app, "cabals-board-row-user-2")
        XCTAssertTrue(secondRow.waitForExistence(timeout: 5), "the second board row should exist")
        XCTAssertTrue(
            secondRow.label.hasPrefix("Rank 2,"),
            "every other row speaks its rank too, got: \(secondRow.label)"
        )
    }

    @MainActor
    func testACabalsListStillLoadingShowsPlaceholdersNotAnError() throws {
        let app = launchApp(scenario: "cabalsLoading")

        let root = anyElement(app, "cabals-root")
        XCTAssertTrue(root.waitForExistence(timeout: 10), "cabals-root should exist after launch")

        XCTAssertTrue(
            anyElement(app, "cabals-list-loading").waitForExistence(timeout: 10),
            "a cabals list that has not arrived yet should show placeholder cards"
        )
        XCTAssertFalse(app.buttons["cabals-list-retry"].exists, "nothing has failed yet")
        XCTAssertFalse(anyElement(app, "cabals-list-empty").exists, "the list is unknown, not empty")

        XCTAssertTrue(
            reveal(app, "cabals-board-list-loading").waitForExistence(timeout: 10),
            "a board that has not arrived yet should show placeholder rows"
        )
        XCTAssertFalse(anyElement(app, "cabals-board-list-error").exists, "the board has not failed yet")
        XCTAssertFalse(anyElement(app, "cabals-board-empty").exists, "the board is unknown, not empty")

        attachScreenshot(app, name: "08-cabals-loading")
    }

    @MainActor
    func testCreatingACabalReplacesTheFormWithTheNewCabal() throws {
        let app = launchApp()

        let plus = app.buttons["cabals-new-button"]
        XCTAssertTrue(plus.waitForExistence(timeout: 10), "the + button should exist")
        plus.tap()
        XCTAssertFalse(anyElement(app, "new-cabal-create-row").exists, "the + opens the form, not a sheet")

        let nameField = app.textFields["create-group-name"]
        XCTAssertTrue(nameField.waitForExistence(timeout: 10), "the Start a cabal form should be pushed")
        nameField.tap()
        nameField.typeText("Lunch money\n")

        let submit = app.buttons["create-group-submit"]
        XCTAssertTrue(
            scrollUntilHittable(app, submit),
            "Create cabal button should be reachable on the Start a cabal form"
        )
        submit.tap()

        let header = anyElement(app, "cabal-header-name")
        XCTAssertTrue(header.waitForExistence(timeout: 10), "the new cabal should be the top screen")
        XCTAssertEqual(header.label, "Lunch money", "the top screen should be the cabal just created")
        XCTAssertFalse(
            app.textFields["create-group-name"].exists,
            "the Start a cabal form must be gone, not underneath the cabal"
        )

        attachScreenshot(app, name: "09-created-cabal")

        app.navigationBars.buttons.element(boundBy: 0).tap()
        XCTAssertTrue(
            anyElement(app, "cabals-root").waitForExistence(timeout: 8),
            "Back from a new cabal should land on the Cabals tab"
        )
        XCTAssertFalse(
            app.textFields["create-group-name"].exists,
            "Back must not land on an armed Start a cabal form"
        )
    }

    @MainActor
    func testQAPotCardPushesDetail() throws {
        let app = launchApp()

        let card = anyElement(app, "cabals-list-card-\(qaPotID)")
        XCTAssertTrue(card.waitForExistence(timeout: 10), "the QA pot card should exist")
        card.tap()

        XCTAssertTrue(
            app.navigationBars.buttons.element(boundBy: 0).waitForExistence(timeout: 8),
            "tapping the card should push a detail screen with a back button"
        )
        let header = anyElement(app, "cabal-header-name")
        XCTAssertTrue(header.waitForExistence(timeout: 10), "the detail screen should show the cabal it opened")
        XCTAssertEqual(header.label, "QA pot", "the detail screen should be the card's cabal")
    }

    @MainActor
    func testFilterWithNoMatchOffersPublicSearch() throws {
        let app = launchApp()

        let filter = app.textFields["cabals-filter-field"]
        XCTAssertTrue(filter.waitForExistence(timeout: 10), "the tab should filter your cabals")
        filter.tap()
        filter.typeText("zzzz")

        XCTAssertTrue(anyElement(app, "cabals-list-no-match").waitForExistence(timeout: 5), "no cabal matches zzzz")
        let link = app.buttons["cabals-search-public"]
        XCTAssertTrue(link.waitForExistence(timeout: 5), "a no-match filter offers a public search")
        link.tap()

        let field = app.textFields["cabals-search-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 8), "Discover cabals should open")
        XCTAssertEqual(field.value as? String, "zzzz", "Discover keeps the typed text")
        attachScreenshot(app, name: "10-discover")
    }

    @MainActor
    func testPublicSearchLinkIsAlwaysVisible() throws {
        let app = launchApp()

        let link = app.buttons["cabals-search-public"]
        XCTAssertTrue(link.waitForExistence(timeout: 10), "the tab always offers the public search")
        XCTAssertEqual(link.label, "Search public cabals", "an empty field offers the plain public search")

        let filter = app.textFields["cabals-filter-field"]
        filter.tap()
        filter.typeText("QA")
        XCTAssertTrue(link.waitForExistence(timeout: 5), "the public search stays while the filter matches")
        XCTAssertEqual(link.label, "Search public cabals for “QA”", "the link carries the typed text")
    }

    @MainActor
    func testSearchKeyOpensPublicSearch() throws {
        let app = launchApp()

        let filter = app.textFields["cabals-filter-field"]
        XCTAssertTrue(filter.waitForExistence(timeout: 10), "the tab has a search field")
        filter.tap()
        filter.typeText("zzzz\n")

        let field = app.textFields["cabals-search-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 8), "the Search key opens Discover cabals")
        XCTAssertEqual(field.value as? String, "zzzz", "Discover keeps the typed text")
    }

    @MainActor
    func testLeaderboardFiltersApplyAndResetThenRankInfo() throws {
        let app = launchApp()

        let filter = reveal(app, "cabals-board-filter")
        XCTAssertTrue(filter.waitForExistence(timeout: 10), "the leaderboard has a filter round")
        filter.tap()
        let sort = app.buttons["Sort by, Return %"]
        XCTAssertTrue(sort.waitForExistence(timeout: 5), "the sheet has a Sort by select")
        XCTAssertTrue(
            sort.isEnabled && app.buttons["Membership, All"].isEnabled
                && app.buttons["Minimum members, Any cabal size"].isEnabled, "the three selects are enabled")
        sort.tap()
        app.buttons["Name A–Z"].tap()
        app.buttons["cabals-board-filter-apply"].tap()
        XCTAssertFalse(app.buttons["cabals-board-filter-apply"].waitForExistence(timeout: 2), "Apply closes the sheet")
        XCTAssertEqual(filter.value as? String, "Filters on", "the filter round shows a filter is on")

        filter.tap()
        app.buttons["cabals-board-filter-reset"].tap()
        XCTAssertFalse(app.buttons["cabals-board-filter-reset"].waitForExistence(timeout: 2), "Reset closes the sheet")
        XCTAssertFalse((filter.value as? String ?? "").contains("Filters on"), "Reset clears the filters")

        let info = reveal(app, "cabals-board-rank-info")
        info.tap()
        XCTAssertTrue(app.staticTexts["About this ranking"].waitForExistence(timeout: 5), "the rank info sheet opens")
    }
}
