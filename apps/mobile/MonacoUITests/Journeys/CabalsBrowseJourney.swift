import XCTest

enum CabalsBrowseJourney {
    static let id = "cabals/browse"
    static let version = 6

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

    static func card(_ app: XCUIApplication, named name: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabal-cover-card-' AND label CONTAINS %@", name)
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
                app.textFields["cabals-filter-field"].waitForExistence(timeout: screenTimeout),
                "S1.1: no filter field")
            XCTAssertTrue(
                card(app, named: mine(run: run)).waitForExistence(timeout: screenTimeout),
                "S1.1: no Your cabals card for \(mine(run: run))")
            XCTAssertTrue(app.staticTexts["Your cabals"].exists, "S1.1: no Your cabals header")
            JoinJourney.snap(app, "S1-A-cabals-tab")
        }

        recorder.step("S1.2", "open a card") {
            card(app, named: mine(run: run)).tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-header-name"), containing: mine(run: run), timeout: screenTimeout),
                "S1.2: the cabal screen for \(mine(run: run)) did not show"
            )
        }

        recorder.step("S1.3", "search for the request cabal") {
            app.tab("Cabals").tap()
            JoinJourney.search(app, for: ask(run: run), step: "S1.3")
            assertResult(app, named: ask(run: run), reads: "1 member · Approval required", step: "S1.3")
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

    static func seesTopCabals(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "the leaderboard lists ranked cabals") {
            app.tab("Cabals").tap()
            let board = app.element("cabals-board")
            app.scrollIntoReach(board)
            XCTAssertTrue(board.waitForExistence(timeout: screenTimeout), "S3.1: no Cabal leaderboard section")
            XCTAssertTrue(app.staticTexts["Cabal leaderboard"].exists, "S3.1: no Cabal leaderboard header")
            XCTAssertTrue(app.staticTexts["Public"].exists, "S3.1: the leaderboard is not marked Public")
            let list = app.element("cabals-board-list")
            let empty = app.element("cabals-board-empty")
            XCTAssertNotNil(
                app.waitForFirst(of: [list, empty], timeout: screenTimeout),
                "S3.1: the leaderboard shows neither ranked cabals nor the empty state within \(Int(screenTimeout)) s"
            )
            let rows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabals-board-row-'"))
            XCTAssertLessThanOrEqual(rows.count, 3, "S3.1: the leaderboard lists more than three rows")
        }
    }
}
