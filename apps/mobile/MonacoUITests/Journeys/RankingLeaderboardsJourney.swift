import XCTest

enum RankingLeaderboardsJourney {
    static let id = "ranking/leaderboards"
    static let version = 2

    struct Seed {
        let cabalID: String
        let cabalName: String
        let memberID: String
        let memberName: String

        static func handedOff() throws -> Seed {
            Seed(
                cabalID: try JourneyHandoff.read("cabalID"),
                cabalName: try JourneyHandoff.read("cabalName"),
                memberID: try JourneyHandoff.read("memberID"),
                memberName: try JourneyHandoff.read("memberName"))
        }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func text(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.staticTexts[label].firstMatch
    }

    static func topInvestors(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "Home shows Top investors") {
            app.tab("Home").tap()
            let header = text(app, "Top investors")
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 15), "S1.1: no \"Top investors\" on Home within 15 s")
        }

        recorder.step("S1.2", "the range chips 1H to All") {
            for range in ["1H", "1D", "1W", "1M", "All"] {
                XCTAssertTrue(
                    app.buttons[range].firstMatch.waitForExistence(timeout: 10),
                    "S1.2: no \"\(range)\" chip on Top investors (known failure, #617 #619 #699)")
            }
        }

        recorder.step("S1.3", "the Everyone and Friends segment") {
            XCTAssertTrue(
                app.buttons["Everyone"].firstMatch.waitForExistence(timeout: 10)
                    && app.buttons["Friends"].firstMatch.exists,
                "S1.3: no \"Everyone\" / \"Friends\" segment (known failure, #658)")
        }

        recorder.step("S1.4", "a ranked row opens the shared cabals") {
            let row = app.element("home-leaderboard-row-\(seed.memberID)")
            app.scrollIntoReach(row)
            XCTAssertTrue(
                row.waitForExistence(timeout: 10),
                "S1.4: no ranked row for \(seed.memberName) (known failure, #617 #619)")
            row.tap()
            XCTAssertTrue(
                app.element("user-profile-group-\(seed.cabalID)").waitForExistence(timeout: 15),
                "S1.4: the profile does not list \(seed.cabalName) as shared (known failure, #699)")
        }
    }

    static func memberBoard(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        let memberRows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-member-'"))
            .matching(NSPredicate(format: "NOT (identifier BEGINSWITH 'cabal-member-board')"))
        let memberRow = app.element("cabal-member-\(seed.memberID)")

        recorder.step("S2.1", "open the seeded cabal") {
            JoinJourney.openBySearch(app, seed.cabalName, step: "S2.1")
        }

        recorder.step("S2.2", "the Leaderboard lists three members with You") {
            let header = text(app, "Leaderboard")
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 15), "S2.2: no \"Leaderboard\" within 15 s")
            app.scrollIntoReach(memberRow)
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(15) { memberRows.count == 3 },
                "S2.2: the Leaderboard has \(memberRows.count) member rows, want 3")
            XCTAssertTrue(
                memberRows.matching(NSPredicate(format: "label CONTAINS 'You'")).firstMatch.exists,
                "S2.2: no member row labelled \"You\"")
        }

        recorder.step("S2.3", "a member row opens their profile") {
            memberRow.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("user-profile-name"), containing: seed.memberName, timeout: 15),
                "S2.3: the profile for \(seed.memberName) did not show within 15 s")
        }

        recorder.step("S2.4", "the members are ranked by return") {
            app.navigationBars.buttons.element(boundBy: 0).tap()
            app.scrollIntoReach(memberRow)
            XCTAssertTrue(memberRow.waitForExistence(timeout: 10), "S2.4: the member board was gone after going back")
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(10) {
                    !app.element("cabal-member-board-coming").exists && memberRow.label.contains("%")
                },
                "S2.4: the members carry no return and the board says rankings come soon (known failure, #617 #619)")
        }
    }

    static func topCabals(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let board = app.element("cabals-board")

        recorder.step("S3.1", "the Cabals tab shows Top cabals") {
            app.tab("Cabals").tap()
            app.scrollIntoReach(board)
            XCTAssertTrue(board.waitForExistence(timeout: 15), "S3.1: no Top cabals board within 15 s")
            XCTAssertTrue(text(app, "Top cabals").exists, "S3.1: no \"Top cabals\" header")
            XCTAssertTrue(
                text(app, "Ranked by return across everyone on Monaco").exists,
                "S3.1: no \"Ranked by return across everyone on Monaco\"")
        }

        recorder.step("S3.2", "Top cabals ranks cabals or says none has funded") {
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(10) { !app.element("cabals-board-coming").exists },
                "S3.2: Top cabals says rankings come soon, with no rows and no empty copy (known failure, #617 #619)")
        }
    }
}
