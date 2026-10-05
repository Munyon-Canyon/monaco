import XCTest

enum CabalsBrowseJourney {
    static let id = "cabals/browse"
    static let version = 2

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func mine(run: String) -> String { "QA mine \(run)" }
    static func open(run: String) -> String { "QA open \(run)" }
    static func ask(run: String) -> String { "QA ask \(run)" }

    static func enterButton(_ app: XCUIApplication, label: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-search-enter-' AND label == %@", label)
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
        recorder.step("S1.1", "open the Cabals tab and see your cards") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                JoinJourney.searchField(app).waitForExistence(timeout: screenTimeout), "S1.1: no search field")
            let card = app.buttons.matching(
                NSPredicate(format: "identifier BEGINSWITH 'cabals-list-card-' AND label CONTAINS %@", mine(run: run))
            ).firstMatch
            XCTAssertTrue(
                card.waitForExistence(timeout: screenTimeout), "S1.1: no Your cabals card for \(mine(run: run))")
            XCTAssertTrue(app.staticTexts["Your cabals"].exists, "S1.1: no Your cabals header")
            JoinJourney.snap(app, "S1-A-cabals-tab")
        }

        recorder.step("S1.2", "open a card") {
            app.buttons.matching(
                NSPredicate(format: "identifier BEGINSWITH 'cabals-list-card-' AND label CONTAINS %@", mine(run: run))
            ).firstMatch.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-header-name"), containing: mine(run: run), timeout: screenTimeout),
                "S1.2: the cabal screen for \(mine(run: run)) did not show"
            )
        }

        recorder.step("S1.3", "search for the open cabal") {
            app.tab("Cabals").tap()
            JoinJourney.search(app, for: open(run: run), step: "S1.3")
            assertResult(app, named: open(run: run), reads: "1 member · Open", step: "S1.3")
            XCTAssertTrue(
                enterButton(app, label: "Join \(open(run: run))").waitForExistence(timeout: 10),
                "S1.3: no Join on the open cabal's row"
            )
        }

        recorder.step("S1.4", "join from the row") {
            enterButton(app, label: "Join \(open(run: run))").tap()
            JoinJourney.waitForToast(app, "You're in.", step: "S1.4")
        }

        recorder.step("S1.5", "search for the request cabal") {
            app.tab("Cabals").tap()
            JoinJourney.search(app, for: ask(run: run), step: "S1.5")
            assertResult(app, named: ask(run: run), reads: "1 member · By request", step: "S1.5")
            XCTAssertTrue(
                enterButton(app, label: "Ask to join \(ask(run: run))").waitForExistence(timeout: 10),
                "S1.5: no Request on the request cabal's row"
            )
        }

        recorder.step("S1.6", "ask from the row") {
            enterButton(app, label: "Ask to join \(ask(run: run))").tap()
            JoinJourney.waitForToast(app, "Request sent. You'll be in once the creator says yes.", step: "S1.6")
            XCTAssertTrue(
                app.element("cabals-search-requested").waitForExistence(timeout: 10),
                "S1.6: the row does not read Request sent"
            )
        }
    }

    static func seesReturnChart(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "the return chart draws your cabals") {
            app.tab("Cabals").tap()
            let chart = app.element("cabals-value-chart")
            XCTAssertTrue(chart.waitForExistence(timeout: screenTimeout), "S2.1: no Your cabals' return section")
            XCTAssertFalse(
                app.element("cabals-value-chart-coming").exists,
                "S2.1: the chart reads Your cabals' return shows up here soon. (known failure, #660)"
            )
        }
    }

    static func seesTopCabals(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "Top cabals lists ranked cabals") {
            app.tab("Cabals").tap()
            let board = app.element("cabals-board")
            app.scrollIntoReach(board)
            XCTAssertTrue(board.waitForExistence(timeout: screenTimeout), "S3.1: no Top cabals section")
            XCTAssertTrue(app.staticTexts["Top cabals"].exists, "S3.1: no Top cabals header")
            XCTAssertFalse(
                app.element("cabals-board-coming").exists,
                "S3.1: Top cabals reads Rankings show up here soon. (known failure, #699 and #617)"
            )
        }
    }
}
