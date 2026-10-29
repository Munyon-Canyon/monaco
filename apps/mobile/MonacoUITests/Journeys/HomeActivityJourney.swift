import XCTest

enum HomeActivityJourney {
    static let id = "home/activity"
    static let version = 2

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func opensCatchUp(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let waiting = try JourneyHandoff.read("proposalID")
        let result = try JourneyHandoff.read("resultProposalID")
        let reply = try JourneyHandoff.read("replyID")
        let bell = app.element("home-notifications-button")
        let subline = app.element("catch-up-subline")
        let resultRow = app.element("catch-up-row-proposal:\(result)")

        recorder.step("S1.1", "the bell shows the count and opens Catch up") {
            app.tab("Home").tap()
            XCTAssertTrue(JourneyDiagnostics.wait(for: bell, in: app, timeout: 15), "S1.1: no bell on Home")
            let counted = NSPredicate(format: "label == %@", "Catch up, 3 new")
            let countedBell = XCTNSPredicateExpectation(predicate: counted, object: bell)
            XCTAssertEqual(
                XCTWaiter().wait(for: [countedBell], timeout: 15), .completed,
                "S1.1: the bell reads '\(bell.label)', not 'Catch up, 3 new'")
            bell.tap()
            XCTAssertTrue(app.navigationBars["Catch up"].waitForExistence(timeout: 15), "S1.1: Catch up did not open")
            XCTAssertTrue(JourneyDiagnostics.wait(for: subline, in: app, timeout: 15), "S1.1: no catch-up-subline")
            XCTAssertTrue(
                subline.label.hasPrefix("3 to catch up"),
                "S1.1: the line reads '\(subline.label)', not '3 to catch up'")
        }

        recorder.step("S1.2", "the three sections list their items") {
            for (identifier, title) in [
                ("catch-up-section-waiting", "Waiting on you"),
                ("catch-up-section-replies", "Replies and mentions"),
                ("catch-up-section-results", "Results"),
            ] {
                let header = app.element(identifier)
                XCTAssertTrue(header.waitForExistence(timeout: 10), "S1.2: no \(identifier)")
                XCTAssertTrue(
                    header.label.contains(title),
                    "S1.2: \(identifier) reads '\(header.label)', not '\(title)'")
            }
            XCTAssertTrue(
                app.element("catch-up-row-proposal:\(waiting)").exists,
                "S1.2: the waiting proposal row is missing")
            XCTAssertTrue(app.element("catch-up-vote-proposal:\(waiting)").exists, "S1.2: the Vote button is missing")
            XCTAssertTrue(app.element("catch-up-row-comment:\(reply)").exists, "S1.2: the reply row is missing")
            XCTAssertTrue(resultRow.exists, "S1.2: the result row is missing")
        }

        clearsAndComesBack(app, waiting: waiting, resultRow: resultRow, recorder: recorder)
    }

    static func clearsAndComesBack(
        _ app: XCUIApplication, waiting: String, resultRow: XCUIElement, recorder: JourneyRecorder
    ) {
        let bell = app.element("home-notifications-button")
        let subline = app.element("catch-up-subline")

        recorder.step("S1.3", "swiping the result row left to right clears it") {
            resultRow.swipeRight()
            let clear = app.buttons["Clear"]
            XCTAssertTrue(clear.waitForExistence(timeout: 10), "S1.3: swiping the result row shows no Clear")
            clear.tap()
            let gone = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: resultRow)
            XCTAssertEqual(
                XCTWaiter().wait(for: [gone], timeout: 10), .completed,
                "S1.3: the result row is still in Results")
            let two = XCTNSPredicateExpectation(
                predicate: NSPredicate(format: "label BEGINSWITH %@", "2 to catch up"), object: subline)
            XCTAssertEqual(
                XCTWaiter().wait(for: [two], timeout: 10), .completed,
                "S1.3: the line reads '\(subline.label)', not '2 to catch up'")
        }

        recorder.step("S1.4", "Vote opens the hold sheet and closing it returns to Catch up") {
            let vote = app.element("catch-up-vote-proposal:\(waiting)")
            XCTAssertTrue(vote.waitForExistence(timeout: 10), "S1.4: no Vote button")
            vote.tap()
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.element("vote-hold-sheet"), in: app, timeout: 15),
                "S1.4: the hold sheet did not open")
            app.element("vote-hold-close").tap()
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: subline, in: app, timeout: 15),
                "S1.4: Catch up did not come back")
        }

        recorder.step("S1.5", "the bell on Home reads the new count") {
            app.tapBack()
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: bell, in: app, timeout: 10),
                "S1.5: the bell did not come back on Home")
            let two = XCTNSPredicateExpectation(
                predicate: NSPredicate(
                    format: "label == %@",
                    "Catch up, 2 new"), object: bell)
            XCTAssertEqual(
                XCTWaiter().wait(for: [two], timeout: 15), .completed,
                "S1.5: the bell reads '\(bell.label)', not 'Catch up, 2 new'")
        }
    }
}
