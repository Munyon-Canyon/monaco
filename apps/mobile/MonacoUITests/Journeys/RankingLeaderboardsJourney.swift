import XCTest

enum RankingLeaderboardsJourney {
    static let id = "ranking/leaderboards"
    static let version = 8

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func text(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.staticTexts[label].firstMatch
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func openPage(_ app: XCUIApplication, step: String) {
        app.tab("Home").tap()
        let row = app.element("home-leaderboard-row")
        app.scrollIntoReach(row)
        XCTAssertTrue(row.waitForExistence(timeout: 15), "\(step): no home-leaderboard-row on Home within 15 s")
        row.tap()
        XCTAssertTrue(
            app.element("leaderboard-page").waitForExistence(timeout: 15),
            "\(step): the Home row did not open leaderboard-page")
    }

    static func home(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "Home's row names the leader and opens the Leaderboard") {
            app.tab("Home").tap()
            let row = app.element("home-leaderboard-row")
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.1: no home-leaderboard-row on Home within 15 s")
            XCTAssertTrue(
                waitUntil(15) { row.label.contains(" leads") && row.label.contains("This month \u{00B7} you are #") },
                "S1.1: the row reads '\(row.label)', not \"… leads · …%\" and \"This month · you are #n of m\"")
            row.tap()
            XCTAssertTrue(
                app.element("leaderboard-page").waitForExistence(timeout: 15),
                "S1.1: the row did not open leaderboard-page")
            XCTAssertTrue(
                app.navigationBars["Leaderboard"].waitForExistence(timeout: 10),
                "S1.1: the page is not titled \"Leaderboard\"")
        }

        recorder.step("S1.2", "Month is selected and People is the first tab") {
            for period in ["week", "month", "all"] {
                XCTAssertTrue(
                    app.element("leaderboard-period-\(period)").waitForExistence(timeout: 10),
                    "S1.2: no leaderboard-period-\(period)")
            }
            XCTAssertTrue(
                app.element("leaderboard-period-month").isSelected, "S1.2: Month is not the selected period")
            for tab in ["people", "cabals", "friends"] {
                XCTAssertTrue(
                    app.element("leaderboard-tab-\(tab)").exists, "S1.2: no leaderboard-tab-\(tab)")
            }
            XCTAssertTrue(
                app.element("leaderboard-tab-people").isSelected, "S1.2: People is not the selected tab")
        }

        recorder.step("S1.3", "the You card, Top three and Around you") {
            let card = app.element("leaderboard-you-card")
            XCTAssertTrue(card.waitForExistence(timeout: 15), "S1.3: no leaderboard-you-card within 15 s")
            XCTAssertTrue(
                card.label.contains("#") && card.label.contains(" people"),
                "S1.3: the You card reads '\(card.label)', not \"#n of m people\"")
            XCTAssertTrue(text(app, "Top three").exists, "S1.3: no \"Top three\"")
            let podium = app.descendants(matching: .any).matching(
                NSPredicate(format: "identifier BEGINSWITH 'leaderboard-podium-'"))
            XCTAssertEqual(podium.count, 3, "S1.3: Top three shows \(podium.count) rows, not 3")
            XCTAssertTrue(text(app, "Around you").exists, "S1.3: no \"Around you\"")
            let me = app.element("leaderboard-row-\(seed.meID)")
            app.scrollIntoReach(me)
            XCTAssertTrue(me.waitForExistence(timeout: 10), "S1.3: no leaderboard-row for A under Around you")
            XCTAssertTrue(
                me.label.contains("percent") && me.label.contains("of ") && me.label.contains(" right"),
                "S1.3: A's row reads '\(me.label)', not a rate and \"n of m right\"")
        }

        recorder.step("S1.4", "a ranked viewer sees no Not ranked yet list") {
            let section = app.element("leaderboard-section-unranked")
            let row = app.element("leaderboard-row-\(seed.unrankedID)")
            _ = row.waitForExistence(timeout: 3)
            XCTAssertFalse(section.exists, "S1.4: A is ranked, yet the board shows a \"Not ranked yet\" section")
            XCTAssertFalse(row.exists, "S1.4: the member under the five-vote floor has a row on the board")
        }
    }

    struct Seed {
        let meID: String
        let unrankedID: String

        static func handedOff() throws -> Seed {
            Seed(meID: try JourneyHandoff.read("meID"), unrankedID: try JourneyHandoff.read("unrankedID"))
        }
    }

    static func cabalsTab(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "the Cabals tab lists cabals with a rate and a record") {
            openPage(app, step: "S2.1")
            app.element("leaderboard-tab-cabals").tap()
            XCTAssertTrue(
                waitUntil(15) { app.element("leaderboard-tab-cabals").isSelected },
                "S2.1: leaderboard-tab-cabals did not become selected")
            let rows = app.descendants(matching: .any).matching(
                NSPredicate(
                    format: "identifier BEGINSWITH 'leaderboard-row-' OR identifier BEGINSWITH 'leaderboard-podium-'"))
            XCTAssertTrue(
                waitUntil(15) { rows.count > 0 },
                "S2.1: the Cabals tab shows no cabal row within 15 s")
            let first = rows.element(boundBy: 0)
            XCTAssertTrue(
                first.label.contains("percent") && first.label.contains(" right"),
                "S2.1: the first cabal row reads '\(first.label)', not a rate and a record")
        }
    }

    static func friends(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "Friends with no follows is empty") {
            openPage(app, step: "S3.1")
            app.element("leaderboard-tab-friends").tap()
            XCTAssertTrue(
                app.element("leaderboard-empty").waitForExistence(timeout: 15),
                "S3.1: the Friends tab shows no empty state within 15 s")
        }
    }
}
