import XCTest

enum CabalsBrowseJourney {
    static let id = "cabals/browse"
    static let version = 9

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
            XCTAssertTrue(
                tile(app, named: mine(run: run)).waitForExistence(timeout: screenTimeout),
                "S1.1: no Your cabals tile for \(mine(run: run))")
            XCTAssertTrue(app.staticTexts["Your cabals"].exists, "S1.1: no Your cabals header")
            XCTAssertTrue(app.element("cabals-new-tile").exists, "S1.1: no New cabal tile")
            JoinJourney.snap(app, "S1-A-cabals-tab")
        }

        recorder.step("S1.2", "open a tile") {
            tile(app, named: mine(run: run)).tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-header-name"), containing: mine(run: run), timeout: screenTimeout),
                "S1.2: the cabal screen for \(mine(run: run)) did not show"
            )
        }

        recorder.step("S1.3", "search for the request cabal") {
            app.waitForToastGone()
            app.tab("Cabals").tap()
            CabalsDiscoverJourney.openFind(app, step: "S1.3")
            JoinJourney.search(app, for: ask(run: run), step: "S1.3")
            assertResult(app, named: ask(run: run), reads: "1 member", step: "S1.3")
            XCTAssertTrue(
                enterButton(app, label: "Request to join \(ask(run: run))").waitForExistence(timeout: 10),
                "S1.3: no Request on the request cabal's row"
            )
        }

        recorder.step("S1.4", "ask from the row") {
            enterButton(app, label: "Request to join \(ask(run: run))").tap()
            JoinJourney.waitForToast(app, "Request sent. You'll be in once the creator says yes.", step: "S1.4")
            XCTAssertTrue(
                app.element("cabals-search-requested").waitForExistence(timeout: 10),
                "S1.4: the row does not read Request sent"
            )
        }
    }

    static func holdsATile(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "press and hold a tile") {
            if JoinJourney.searchField(app).exists { app.tapBack() }
            app.tab("Cabals").tap()
            let held = tile(app, named: mine(run: run))
            XCTAssertTrue(held.waitForExistence(timeout: screenTimeout), "S2.1: no tile for \(mine(run: run))")
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
