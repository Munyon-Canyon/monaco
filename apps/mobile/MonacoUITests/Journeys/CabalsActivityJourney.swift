import XCTest

enum CabalsActivityJourney {
    static let id = "cabals/activity"
    static let version = 9

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
        app.openCabal(named: name)
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

        recorder.step("S1.2", "Treasury history opens with the failed trade first") {
            openHistory(app, step: "S1.2")
            let section = app.element("cabal-activity")
            XCTAssertTrue(section.waitForExistence(timeout: screenTimeout), "S1.2: no activity list within 15 s")
            let newest = row(app, failed)
            app.scrollIntoReach(newest)
            XCTAssertTrue(newest.waitForExistence(timeout: screenTimeout), "S1.2: no row for the failed trade")
            XCTAssertTrue(newest.label.hasPrefix("Bought"), "S1.2: the failed row's title: \(newest.label)")
            XCTAssertTrue(newest.label.contains("Failed"), "S1.2: the failed row has no Failed: \(newest.label)")
            JoinJourney.snap(app, "S1-A-activity-section")
        }

        recorder.step("S1.3", "Treasury history lists every row") {
            let rows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-activity-row-'"))
            let deadline = Date().addingTimeInterval(10)
            while rows.count < 6 && Date() < deadline {
                RunLoop.current.run(until: Date().addingTimeInterval(0.25))
            }
            XCTAssertEqual(rows.count, 6, "S1.3: the Treasury history rows")
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

        backOutOfReceipts(app, recorder: recorder)
    }

    static func backOutOfReceipts(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.5", "Back from the receipt returns to Treasury history") {
            app.navigationBars["Transaction"].buttons.element(boundBy: 0).tap()
            XCTAssertTrue(
                app.navigationBars["Treasury history"].waitForExistence(timeout: 10), "S1.5: no Treasury history")
        }

        recorder.step("S1.6", "open a Money added receipt from the history") {
            openDepositReceipt(app, step: "S1.6")
        }

        recorder.step("S1.7", "Back returns straight to the history") {
            backToHistory(app, step: "S1.7")
        }

        recorder.step("S1.8", "open a Money added receipt again") {
            openDepositReceipt(app, step: "S1.8")
        }

        recorder.step("S1.9", "Back again lands on one history screen") {
            backToHistory(app, step: "S1.9")
        }

        recorder.step("S1.10", "Back from the history and the cabal leaves the cabal") {
            app.navigationBars["Treasury history"].buttons.element(boundBy: 0).tap()
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 10),
                "S1.10: Back from the history did not reach the cabal")
            app.navigationBars.buttons.element(boundBy: 0).tap()
            XCTAssertTrue(
                app.tab("Cabals").waitForExistence(timeout: 10), "S1.10: Back from the cabal did not leave it")
            XCTAssertFalse(app.element("cabal-header-name").exists, "S1.10: a second cabal screen was under the first")
        }
    }

    static func retryFailedTrade(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws {
        let failed = try JourneyHandoff.read("failed_trade")

        recorder.step("S2.1", "open the cabal") {
            openCabal(app, run: run, step: "S2.1")
        }

        recorder.step("S2.2", "open the failed trade's receipt") {
            openHistory(app, step: "S2.2")
            let target = row(app, failed)
            app.scrollIntoReach(target)
            XCTAssertTrue(target.waitForExistence(timeout: screenTimeout), "S2.2: no row for the failed trade")
            target.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-txn-status"), containing: "Failed", timeout: 10),
                "S2.2: the status is not Failed"
            )
        }

        recorder.step("S2.3", "the receipt offers Retry") {
            XCTAssertTrue(
                app.buttons["cabal-txn-retry"].waitForExistence(timeout: 5),
                "S2.3: the failed trade's receipt has no Retry (known failure, #654)"
            )
        }
    }

    static func openDepositReceipt(_ app: XCUIApplication, step: String) {
        let deposit = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Money added'")).firstMatch
        app.scrollIntoReach(deposit)
        XCTAssertTrue(deposit.waitForExistence(timeout: screenTimeout), "\(step): no Money added row")
        deposit.tap()
        XCTAssertTrue(
            app.navigationBars["Transaction"].waitForExistence(timeout: 10), "\(step): no receipt within 10 s")
    }

    static func openHistory(_ app: XCUIApplication, step: String) {
        app.openCabalTab("Treasury")
        let history = app.buttons["cabal-treasury-history"]
        app.scrollIntoReach(history)
        XCTAssertTrue(history.waitForExistence(timeout: screenTimeout), "\(step): no Treasury history row")
        history.tap()
        XCTAssertTrue(
            app.navigationBars["Treasury history"].waitForExistence(timeout: 10),
            "\(step): no Treasury history screen within 10 s")
    }

    static func backToHistory(_ app: XCUIApplication, step: String) {
        app.navigationBars["Transaction"].buttons.element(boundBy: 0).tap()
        XCTAssertTrue(
            app.navigationBars["Treasury history"].waitForExistence(timeout: 10),
            "\(step): Back did not land on the history")
        XCTAssertFalse(app.navigationBars["Transaction"].exists, "\(step): the receipt is still up")
        XCTAssertEqual(
            app.sheets.count + app.alerts.count + app.popovers.count + app.menus.count, 0,
            "\(step): Back opened a prompt")
        JoinJourney.snap(app, "S1-A-back-to-history-\(step)")
    }
}
