import XCTest

enum HomeActivityJourney {
    static let id = "home/activity"
    static let version = 2

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func bellCount(_ app: XCUIApplication) -> Int? {
        let label = app.element("home-notifications-button").label
        guard let range = label.range(of: "Catch up, ") else { return nil }
        let digits = label[range.upperBound...].prefix { $0.isNumber }
        return Int(digits)
    }

    static func sublineCount(_ app: XCUIApplication) -> Int? {
        let label = app.element("catch-up-subline").label
        let digits = label.prefix { $0.isNumber }
        return Int(digits)
    }

    static func waitForSubline(_ app: XCUIApplication, count: Int, timeout: TimeInterval) -> Bool {
        JourneyWait.until(timeout: timeout) { sublineCount(app) == count }
    }

    static func opensFromBell(
        _ app: XCUIApplication, waitingKey: String, replyKey: String, resultKey: String, recorder: JourneyRecorder
    ) -> Int {
        let bell = app.element("home-notifications-button")
        var count = 0

        recorder.step("S1.1", "the bell's badge is the Catch up count and opens the screen") {
            app.tab("Home").tap()
            XCTAssertTrue(bell.waitForExistence(timeout: 15), "S1.1: no bell on Home")
            XCTAssertTrue(
                JourneyWait.until(timeout: 15) { (bellCount(app) ?? 0) >= 3 },
                "S1.1: the bell reads \"\(bell.label)\", not \"Catch up, n new\" with n at least 3")
            XCTAssertTrue(
                app.element("home-bell-badge").exists || bell.label.contains("new"),
                "S1.1: the bell shows no badge")
            bell.tap()
            XCTAssertTrue(
                app.element("catch-up-page").waitForExistence(timeout: 15),
                "S1.1: catch-up-page did not show within 15 s")
            XCTAssertTrue(app.navigationBars["Catch up"].exists, "S1.1: the screen is not titled \"Catch up\"")
            let subline = app.element("catch-up-subline")
            XCTAssertTrue(subline.waitForExistence(timeout: 15), "S1.1: no catch-up-subline within 15 s")
            count = sublineCount(app) ?? 0
            XCTAssertGreaterThanOrEqual(count, 3, "S1.1: the subline reads \"\(subline.label)\"")
            XCTAssertTrue(subline.label.contains("to catch up"), "S1.1: the subline reads \"\(subline.label)\"")
        }

        recorder.step("S1.2", "the sections list the waiting, reply and result items") {
            for section in ["waiting", "replies", "results"] {
                XCTAssertTrue(
                    app.element("catch-up-section-\(section)").waitForExistence(timeout: 15),
                    "S1.2: catch-up-section-\(section) did not show within 15 s")
            }
            for key in [waitingKey, replyKey, resultKey] {
                let row = app.element("catch-up-row-\(key)")
                XCTAssertTrue(
                    row.waitForExistence(timeout: 15), "S1.2: catch-up-row-\(key) did not show within 15 s")
            }
            let vote = app.element("catch-up-vote-\(waitingKey)")
            XCTAssertTrue(vote.exists, "S1.2: the waiting row has no Vote button")
            XCTAssertEqual(vote.label, "Vote", "S1.2: the waiting row's button reads \"\(vote.label)\"")
        }

        return count
    }

    static func clearsAResult(_ app: XCUIApplication, resultKey: String, count: Int, recorder: JourneyRecorder) {
        recorder.step("S1.3", "swiping a result row clears it and the count drops") {
            let row = app.element("catch-up-row-\(resultKey)")
            app.scrollIntoReach(row)
            row.swipeRight()
            let clear = app.buttons["Clear"].firstMatch
            if clear.waitForExistence(timeout: 3) { clear.tap() }
            XCTAssertTrue(
                waitForSubline(app, count: count - 1, timeout: 10),
                "S1.3: catch-up-subline reads \"\(app.element("catch-up-subline").label)\", not \"\(count - 1) to catch up\""
            )
            XCTAssertTrue(
                app.element("catch-up-section-cleared").waitForExistence(timeout: 10),
                "S1.3: no \"Cleared today\" section after the clear")
            XCTAssertFalse(app.element("monaco-toast-banner").exists, "S1.3: a toast showed on the clear")
        }
    }

    static func votesFromARow(_ app: XCUIApplication, waitingKey: String, count: Int, recorder: JourneyRecorder) {
        recorder.step("S1.4", "Vote opens the hold sheet and closing it returns to Catch up") {
            let vote = app.element("catch-up-vote-\(waitingKey)")
            app.scrollIntoReach(vote)
            vote.tap()
            XCTAssertTrue(
                app.element("vote-hold-sheet").waitForExistence(timeout: 10),
                "S1.4: vote-hold-sheet did not show within 10 s")
            XCTAssertTrue(
                app.element("vote-hold-row").waitForExistence(timeout: 10),
                "S1.4: the hold sheet shows no vote-hold-row within 10 s")
            app.element("vote-hold-close").tap()
            XCTAssertTrue(
                app.element("catch-up-page").waitForExistence(timeout: 10),
                "S1.4: Catch up did not come back within 10 s")
            XCTAssertTrue(
                waitForSubline(app, count: count - 1, timeout: 10),
                "S1.4: catch-up-subline reads \"\(app.element("catch-up-subline").label)\", not \"\(count - 1) to catch up\""
            )
        }
    }

    static func badgeDrops(_ app: XCUIApplication, count: Int, recorder: JourneyRecorder) {
        recorder.step("S1.5", "back on Home the bell reads the new count") {
            app.tapBack()
            let bell = app.element("home-notifications-button")
            XCTAssertTrue(bell.waitForExistence(timeout: 15), "S1.5: the bell did not come back on Home")
            XCTAssertTrue(app.tab("Home").exists, "S1.5: the tab bar is gone")
            XCTAssertTrue(
                JourneyWait.until(timeout: 15) { bellCount(app) == count - 1 },
                "S1.5: the bell reads \"\(bell.label)\", not \"Catch up, \(count - 1) new\"")
        }
    }
}
