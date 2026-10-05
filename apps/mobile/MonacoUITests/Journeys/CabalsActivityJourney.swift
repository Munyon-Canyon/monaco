import XCTest

enum CabalsActivityJourney {
    static let id = "cabals/activity"
    static let version = 2

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA activity \(run)" }

    static func row(_ app: XCUIApplication, _ activityID: String) -> XCUIElement {
        app.buttons["cabal-activity-row-\(activityID)"]
    }

    static func openCabal(_ app: XCUIApplication, run: String, step: String) {
        let name = cabal(run: run)
        JoinJourney.openBySearch(app, name, step: step)
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: name, timeout: screenTimeout),
            "\(step): the header does not read \(name)"
        )
    }

    static func readAndOpenReceipt(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws {
        let failed = try JourneyHandoff.read("failed_trade")
        let confirmed = try JourneyHandoff.read("confirmed_trade")

        recorder.step("S1.1", "open the cabal") {
            openCabal(app, run: run, step: "S1.1")
        }

        recorder.step("S1.2", "the Activity section shows the failed trade first") {
            let section = app.element("cabal-activity")
            app.scrollIntoReach(section)
            XCTAssertTrue(section.waitForExistence(timeout: screenTimeout), "S1.2: no Activity section within 15 s")
            XCTAssertTrue(app.element("cabal-activity-see-all").exists, "S1.2: the Activity header has no See all")
            let newest = row(app, failed)
            app.scrollIntoReach(newest)
            XCTAssertTrue(newest.waitForExistence(timeout: screenTimeout), "S1.2: no row for the failed trade")
            XCTAssertTrue(newest.label.hasPrefix("Bought"), "S1.2: the failed row's title: \(newest.label)")
            XCTAssertTrue(newest.label.contains("Failed"), "S1.2: the failed row has no Failed: \(newest.label)")
            JoinJourney.snap(app, "S1-A-activity-section")
        }

        recorder.step("S1.3", "See all lists every row") {
            app.element("cabal-activity-see-all").tap()
            XCTAssertTrue(
                app.navigationBars["Activity"].waitForExistence(timeout: 10), "S1.3: no Activity screen within 10 s")
            let rows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-activity-row-'"))
            let deadline = Date().addingTimeInterval(10)
            while rows.count < 6 && Date() < deadline {
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
            }
            XCTAssertEqual(rows.count, 6, "S1.3: the Activity screen's rows")
            XCTAssertTrue(
                app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Money added'")).firstMatch.exists,
                "S1.3: no Money added row"
            )
        }

        recorder.step("S1.4", "open the confirmed trade's receipt") {
            let target = row(app, confirmed)
            app.scrollIntoReach(target)
            target.tap()
            XCTAssertTrue(
                app.navigationBars["Transaction"].waitForExistence(timeout: 10), "S1.4: no receipt within 10 s")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-txn-amount"), containing: "$25.00", timeout: 10),
                "S1.4: the amount: \(app.element("cabal-txn-amount").label)"
            )
            XCTAssertTrue(app.element("cabal-txn-status").label.contains("Done"), "S1.4: the status is not Done")
            XCTAssertTrue(
                app.element("cabal-txn-solscan").label.contains("View on Solscan"), "S1.4: no View on Solscan")
            JoinJourney.snap(app, "S1-A-receipt")
        }
    }

    static func retryFailedTrade(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws {
        let failed = try JourneyHandoff.read("failed_trade")

        recorder.step("S2.1", "open the cabal") {
            openCabal(app, run: run, step: "S2.1")
        }

        recorder.step("S2.2", "open the failed trade's receipt") {
            let target = row(app, failed)
            app.scrollIntoReach(target)
            XCTAssertTrue(target.waitForExistence(timeout: screenTimeout), "S2.2: no row for the failed trade")
            let glyphClearOfInlineRetry = target.coordinate(withNormalizedOffset: CGVector(dx: 0.08, dy: 0.5))
            glyphClearOfInlineRetry.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-txn-status"), containing: "Failed", timeout: 10),
                "S2.2: the status is not Failed"
            )
        }

        recorder.step("S2.3", "the receipt offers Retry") {
            XCTAssertTrue(
                app.buttons["Retry"].waitForExistence(timeout: 5),
                "S2.3: the failed trade's receipt has no Retry (known failure, #654)"
            )
        }
    }
}
