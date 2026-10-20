import XCTest

enum RankingLeaderboardsJourney {
    static let id = "ranking/leaderboards"
    static let version = 6

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func text(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.staticTexts[label].firstMatch
    }

    static func homeLeaderboard(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let category = app.element("home-leaderboard-category")
        let period = app.element("home-leaderboard-period")

        recorder.step("S1.1", "Home shows the Leaderboard") {
            app.tab("Home").tap()
            let header = text(app, "Leaderboard")
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 15), "S1.1: no \"Leaderboard\" on Home within 15 s")
            XCTAssertTrue(app.element("home-people-board").exists, "S1.1: no home-people-board")
            XCTAssertTrue(app.buttons["View all"].firstMatch.exists, "S1.1: no \"View all\" under the Leaderboard")
        }

        recorder.step("S1.2", "the Cabals board for 1M") {
            XCTAssertTrue(category.waitForExistence(timeout: 10), "S1.2: no home-leaderboard-category")
            XCTAssertTrue(category.label.contains("Cabals"), "S1.2: the category reads '\(category.label)'")
            XCTAssertTrue(period.label.contains("1M"), "S1.2: the period reads '\(period.label)'")
            XCTAssertTrue(
                text(app, "Cabal P&L \u{00B7} 1M").waitForExistence(timeout: 10),
                "S1.2: no \"Cabal P&L \u{00B7} 1M\" context line")
        }

        recorder.step("S1.3", "switch to People") {
            category.tap()
            let people = app.buttons["People"].firstMatch
            XCTAssertTrue(people.waitForExistence(timeout: 10), "S1.3: no People option in the category menu")
            people.tap()
            XCTAssertTrue(
                text(app, "Personal P&L \u{00B7} 1M").waitForExistence(timeout: 10),
                "S1.3: no \"Personal P&L \u{00B7} 1M\" context line after choosing People")
            XCTAssertTrue(category.label.contains("People"), "S1.3: the category reads '\(category.label)'")
        }

        recorder.step("S1.4", "the People board ranks investors or says none is ranked") {
            let rows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'home-leaderboard-row-'"))
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(15) {
                    rows.count > 0 || app.element("home-leaderboard-empty").exists
                },
                "S1.4: the People board shows no rows and no empty copy")
            XCTAssertFalse(
                app.element("home-leaderboard-loading").exists, "S1.4: the People board is still loading")
        }
    }

    static func viewAll(_ app: XCUIApplication, recorder: JourneyRecorder) {
        var homeCategory = ""
        var homePeriod = ""
        recorder.step("S4.1", "View all opens the Leaderboard") {
            app.tab("Home").tap()
            let header = text(app, "Leaderboard")
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 15), "S4.1: no \"Leaderboard\" on Home within 15 s")
            let viewAll = app.buttons["View all"].firstMatch
            XCTAssertTrue(viewAll.waitForExistence(timeout: 10), "S4.1: no \"View all\" under the Leaderboard")
            homeCategory = app.element("home-leaderboard-category").label
            homePeriod = app.element("home-leaderboard-period").label
            viewAll.tap()
            XCTAssertTrue(
                app.element("leaderboard-page").waitForExistence(timeout: 15),
                "S4.1: View all did not open the Leaderboard page"
            )
        }

        recorder.step("S4.2", "the Leaderboard tools and rows") {
            XCTAssertTrue(
                app.element("leaderboard-category").waitForExistence(timeout: 10), "S4.2: no leaderboard-category")
            let board = app.element("leaderboard-category").label
            let range = app.element("leaderboard-period").label
            XCTAssertTrue(board == homeCategory, "S4.2: the page reads '\(board)', Home read '\(homeCategory)'")
            XCTAssertTrue(range == homePeriod, "S4.2: the page reads '\(range)', Home read '\(homePeriod)'")
            let rows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'leaderboard-row-'"))
            let outcomes = [app.element("leaderboard-empty"), app.element("leaderboard-stale")]
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(15) {
                    rows.count > 0 || outcomes.contains { $0.exists }
                },
                "S4.2: the Leaderboard shows no rows and no empty copy")
        }
    }

    static func topCabals(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let board = app.element("cabals-board")

        recorder.step("S3.1", "the Cabals tab shows the Cabal leaderboard") {
            app.tab("Cabals").tap()
            app.scrollIntoReach(board)
            XCTAssertTrue(board.waitForExistence(timeout: 15), "S3.1: no Cabal leaderboard within 15 s")
            XCTAssertTrue(text(app, "Cabal leaderboard").exists, "S3.1: no \"Cabal leaderboard\" header")
            XCTAssertTrue(
                text(app, "Cabal P&L \u{00B7} 1M").exists,
                "S3.1: no \"Cabal P&L \u{00B7} 1M\" context line")
        }

        recorder.step("S3.2", "the leaderboard ranks cabals or says none is public") {
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(10) { !app.element("cabals-board-coming").exists },
                "S3.2: the leaderboard says rankings come soon, with no rows and no empty copy")
        }
    }
}
