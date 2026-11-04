import XCTest

enum CabalsBrowseJourney {
    static let id = "cabals/browse"
    static let version = 10

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func mine(run: String) -> String { "QA mine \(run)" }
    static func ask(run: String) -> String { "QA ask \(run)" }

    static func enterButton(_ app: XCUIApplication, label: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-search-enter-' AND label == %@", label)
        ).firstMatch
    }

    static func tile(_ app: XCUIApplication, named name: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-tile-' AND label CONTAINS %@", name)
        ).firstMatch
    }

    static func hint(_ app: XCUIApplication) -> XCUIElement { app.element("cabals-reorder-hint") }

    static func waitForHintGone(_ app: XCUIApplication, timeout: TimeInterval = 5) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while hint(app).exists, Date() < deadline { Thread.sleep(forTimeInterval: 0.25) }
        return !hint(app).exists
    }

    static func assertUpNext(_ app: XCUIApplication, tile mineTile: XCUIElement) {
        let id = mineTile.identifier.replacingOccurrences(of: "cabals-tile-", with: "")
        let row = app.element("cabals-upnext-row-\(id)")
        XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.1: no Up next row for the new cabal")
        XCTAssertTrue(
            app.staticTexts["Just you so far."].exists, "S1.1: the Up next row does not read Just you so far.")
        let action = app.element("cabals-upnext-action-\(id)")
        XCTAssertTrue(action.exists, "S1.1: the Up next row has no action")
        XCTAssertEqual(action.label, "Invite", "S1.1: the Up next action reads \(action.label), not Invite")
    }

    static func assertResult(_ app: XCUIApplication, named name: String, reads caption: String, step: String) {
        let row = JoinJourney.result(app, named: name).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10), "\(step): no search result named \(name) within 10 s")
        XCTAssertEqual(
            JoinJourney.result(app, named: name).count, 1, "\(step): \(name) is in the results more than once")
        XCTAssertTrue(row.label.contains(caption), "\(step): the result does not read \(caption): \(row.label)")
    }

    static func browses(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Cabals tab and see your tiles") {
            app.tab("Cabals").tap()
            let mineTile = tile(app, named: mine(run: run))
            XCTAssertTrue(
                mineTile.waitForExistence(timeout: screenTimeout) || app.revealInGrid(mineTile),
                "S1.1: no Your cabals tile for \(mine(run: run))")
            for _ in 0..<8 where !app.staticTexts["Your cabals"].isHittable { app.swipeDown() }
            XCTAssertTrue(app.staticTexts["Your cabals"].exists, "S1.1: no Your cabals header")
            XCTAssertTrue(app.element("cabals-new-tile").exists, "S1.1: no New cabal tile")
            assertUpNext(app, tile: mineTile)
            if hint(app).exists {
                XCTAssertTrue(
                    app.staticTexts["Hold a cabal for quick actions."].exists,
                    "S1.1: the hint does not read Hold a cabal for quick actions.")
                XCTAssertTrue(
                    app.element("cabals-reorder-hint-dismiss").exists, "S1.1: the hint has no dismiss button")
            }
            JoinJourney.snap(app, "S1-A-cabals-tab")
        }

        recorder.step("S1.2", "dismiss the hold hint and keep it dismissed across a relaunch") {
            let dismiss = app.element("cabals-reorder-hint-dismiss")
            if hint(app).exists, dismiss.exists {
                dismiss.tap()
            }
            XCTAssertTrue(waitForHintGone(app), "S1.2: the hint is still there 5 s after dismissing it")
            app.terminate()
            app.launch()
            XCTAssertTrue(app.tab("Cabals").waitForExistence(timeout: 30), "S1.2: no tab bar after the relaunch")
            app.tab("Cabals").tap()
            XCTAssertTrue(
                app.element("cabals-grid").waitForExistence(timeout: screenTimeout),
                "S1.2: no Cabals grid after the relaunch")
            XCTAssertTrue(waitForHintGone(app), "S1.2: the hint came back after the relaunch")
        }

        recorder.step("S1.3", "open a tile") {
            tile(app, named: mine(run: run)).tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-header-name"), containing: mine(run: run), timeout: screenTimeout),
                "S1.3: the cabal screen for \(mine(run: run)) did not show"
            )
        }

        recorder.step("S1.4", "search for the request cabal") {
            app.waitForToastGone()
            app.tab("Cabals").tap()
            CabalsDiscoverJourney.openFind(app, step: "S1.4")
            JoinJourney.search(app, for: ask(run: run), step: "S1.4")
            assertResult(app, named: ask(run: run), reads: "1 member", step: "S1.4")
            XCTAssertTrue(
                enterButton(app, label: "Request to join \(ask(run: run))").waitForExistence(timeout: 10),
                "S1.4: no Request on the request cabal's row"
            )
        }

        recorder.step("S1.5", "ask from the row") {
            enterButton(app, label: "Request to join \(ask(run: run))").tap()
            JoinJourney.waitForToast(app, "Request sent. You'll be in once the creator says yes.", step: "S1.5")
            XCTAssertTrue(
                app.element("cabals-search-requested").waitForExistence(timeout: 10),
                "S1.5: the row does not read Request sent"
            )
        }
    }

    static func holdsATile(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "press and hold a tile") {
            if JoinJourney.searchField(app).exists { app.tapBack() }
            for _ in 0..<3 where !app.element("cabals-grid").exists {
                app.tab("Cabals").tap()
                _ = app.element("cabals-grid").waitForExistence(timeout: 5)
            }
            let held = tile(app, named: mine(run: run))
            for _ in 0..<4 where !held.isHittable { app.swipeDown() }
            XCTAssertTrue(held.isHittable || app.revealInGrid(held), "S2.1: no tile for \(mine(run: run))")
            held.press(forDuration: 1.0)
            for item in ["Fund", "Propose buy", "Invite", "Share"] {
                XCTAssertTrue(
                    menuItem(app, item).waitForExistence(timeout: 10), "S2.1: the menu has no \(item)")
            }
            XCTAssertFalse(
                app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Vote on'")).firstMatch.exists,
                "S2.1: the menu offers a vote with none open")
        }

        recorder.step("S2.2", "Fund opens the keypad and closing returns to the tab") {
            menuItem(app, "Fund").tap()
            XCTAssertTrue(
                app.element("amount-keypad-screen").waitForExistence(timeout: 10), "S2.2: Fund did not open the keypad")
            app.element("amount-keypad-close").tap()
            XCTAssertTrue(
                app.element("cabals-grid").waitForExistence(timeout: 10), "S2.2: closing did not return to the tab")
        }
    }

    static func menuItem(_ app: XCUIApplication, _ title: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "label == %@", title)).firstMatch
    }
}
