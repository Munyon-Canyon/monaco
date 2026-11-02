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
        XCTAssertTrue(
            anyElement(app, "cabals-root").waitForExistence(timeout: 15),
            "the Cabals tab should open; an unknown scenario or a renamed harness lands on sign-in"
        )
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

    @MainActor
    private func anyTile(_ app: XCUIApplication) -> XCUIElement {
        let predicate = NSPredicate(format: "identifier BEGINSWITH %@", "cabals-tile-")
        return app.descendants(matching: .any).matching(predicate).firstMatch
    }

    @MainActor
    func testLoadingShowsPlaceholderTilesNotBlankSpace() throws {
        let app = launchApp(scenario: "cabalsLoading")

        XCTAssertTrue(anyElement(app, "cabals-header").waitForExistence(timeout: 10), "the header draws while loading")
        let loading = anyElement(app, "cabals-list-loading")
        XCTAssertTrue(loading.waitForExistence(timeout: 10), "a list that has not arrived shows placeholder tiles")
        XCTAssertEqual(loading.label, "Loading your cabals", "the placeholders are spoken as one loading state")
        XCTAssertFalse(app.buttons["cabals-list-retry"].exists, "nothing has failed yet")
        XCTAssertFalse(anyElement(app, "cabals-list-empty").exists, "the list is unknown, not empty")
        XCTAssertFalse(anyTile(app).exists, "no real tile exists before the list arrives")

        attachScreenshot(app, name: "01-cabals-loading")
    }

    @MainActor
    func testLoadedTabShowsTheGridAndNoPlaceholders() throws {
        let app = launchApp()

        XCTAssertTrue(anyElement(app, "cabals-header").waitForExistence(timeout: 10), "the header should exist")
        XCTAssertTrue(app.buttons["cabals-new-button"].exists, "the + button sits in the header")
        XCTAssertTrue(anyElement(app, "cabals-grid").waitForExistence(timeout: 10), "the cabal grid should exist")
        XCTAssertTrue(anyTile(app).waitForExistence(timeout: 10), "a loaded grid draws a tile for each cabal")
        XCTAssertFalse(anyElement(app, "cabals-list-loading").exists, "placeholders are gone once cabals load")
        XCTAssertFalse(anyElement(app, "cabals-list-empty").exists, "a member with cabals is not empty")

        attachScreenshot(app, name: "02-cabals-grid")
    }

    @MainActor
    func testEmptyTabOffersCreateAndExplore() throws {
        let app = launchApp(scenario: "empty")

        XCTAssertTrue(anyElement(app, "cabals-list-empty").waitForExistence(timeout: 10), "an empty list says so")
        XCTAssertTrue(app.staticTexts["Your cabal starts here"].exists, "the empty state has its title")
        XCTAssertTrue(app.buttons["Create a cabal"].exists, "the empty state offers Create a cabal")
        XCTAssertTrue(app.buttons["Explore cabals"].exists, "the empty state offers Explore cabals")
        XCTAssertFalse(anyElement(app, "cabals-list-loading").exists, "an empty list is not loading")
        XCTAssertFalse(app.buttons["cabals-list-retry"].exists, "an empty list has not failed")

        attachScreenshot(app, name: "03-cabals-empty")

        app.buttons["Explore cabals"].tap()
        XCTAssertTrue(
            app.textFields["cabals-search-field"].waitForExistence(timeout: 8),
            "Explore cabals opens Discover's search"
        )
    }

    @MainActor
    func testEmptyTabCreateButtonOpensStartACabal() throws {
        let app = launchApp(scenario: "empty")

        let create = app.buttons["Create a cabal"]
        XCTAssertTrue(create.waitForExistence(timeout: 10), "the empty state offers Create a cabal")
        create.tap()
        XCTAssertTrue(
            app.textFields["create-group-name"].waitForExistence(timeout: 8),
            "Create a cabal opens the Start a cabal form"
        )
    }

    @MainActor
    func testUnloadedCabalsDoNotClaimTheMemberHasNone() throws {
        let app = launchApp(scenario: "cabalsUnavailable")

        XCTAssertTrue(
            app.buttons["cabals-list-retry"].waitForExistence(timeout: 10),
            "an unloaded cabals list should say it failed and offer a retry"
        )
        XCTAssertTrue(app.staticTexts["Couldn't load this view"].exists, "the failure is spelled out")
        XCTAssertFalse(
            anyElement(app, "cabals-list-empty").exists,
            "an unloaded cabals list must not claim 'Your cabal starts here'"
        )
        XCTAssertFalse(anyElement(app, "cabals-list-loading").exists, "a failed list stops showing placeholders")

        attachScreenshot(app, name: "04-cabals-unavailable")
    }

    @MainActor
    func testGridTileOpensItsCabal() throws {
        let app = launchApp()

        let tile = anyTile(app)
        XCTAssertTrue(tile.waitForExistence(timeout: 10), "the grid should have a tile")
        tile.tap()

        XCTAssertTrue(
            anyElement(app, "cabal-header-name").waitForExistence(timeout: 10),
            "tapping a tile opens that cabal"
        )
    }

    @MainActor
    func testNewCabalTileOpensStartACabal() throws {
        let app = launchApp()

        let tile = anyElement(app, "cabals-new-tile")
        XCTAssertTrue(tile.waitForExistence(timeout: 10), "the grid ends with a New cabal tile")
        tile.tap()
        XCTAssertTrue(
            app.textFields["create-group-name"].waitForExistence(timeout: 8),
            "the New cabal tile opens the Start a cabal form"
        )
    }

    @MainActor
    func testPlusMenuOffersStartJoinAndFind() throws {
        let app = launchApp()

        let plus = app.buttons["cabals-new-button"]
        XCTAssertTrue(plus.waitForExistence(timeout: 10), "the + button should exist")
        plus.tap()
        XCTAssertTrue(app.buttons["cabals-menu-start"].waitForExistence(timeout: 5), "the menu offers Start a cabal")
        XCTAssertTrue(app.buttons["cabals-menu-join"].exists, "the menu offers Join with a link")
        XCTAssertTrue(app.buttons["cabals-menu-find"].exists, "the menu offers Find a cabal")
    }

    @MainActor
    func testPlusMenuFindOpensDiscoverSearch() throws {
        let app = launchApp()

        let plus = app.buttons["cabals-new-button"]
        XCTAssertTrue(plus.waitForExistence(timeout: 10), "the + button should exist")
        plus.tap()
        let find = app.buttons["cabals-menu-find"]
        XCTAssertTrue(find.waitForExistence(timeout: 5), "the menu offers Find a cabal")
        find.tap()

        XCTAssertTrue(
            app.textFields["cabals-search-field"].waitForExistence(timeout: 8),
            "Find a cabal opens Discover's search"
        )
        attachScreenshot(app, name: "05-discover")
    }

    @MainActor
    func testCreatingACabalReplacesTheFormWithTheNewCabal() throws {
        let app = launchApp()

        let plus = app.buttons["cabals-new-button"]
        XCTAssertTrue(plus.waitForExistence(timeout: 10), "the + button should exist")
        plus.tap()
        let start = app.buttons["cabals-menu-start"]
        XCTAssertTrue(start.waitForExistence(timeout: 5), "the menu offers Start a cabal")
        start.tap()

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

        attachScreenshot(app, name: "06-created-cabal")

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
}
