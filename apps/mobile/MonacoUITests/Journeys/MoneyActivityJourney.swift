import XCTest

enum MoneyActivityJourney {
    static let id = "money/activity"
    static let version = 2

    static let screenTimeout: TimeInterval = 15
    static let depositKey = "deposit-txn"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA activity \(run)" }

    static func openActivity(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        let settings = app.element("profile-settings-row")
        XCTAssertTrue(settings.waitForExistence(timeout: screenTimeout), "\(step): no Settings on Profile")
        app.scrollIntoReach(settings)
        settings.tap()
        let activity = app.element("settings-activity")
        XCTAssertTrue(activity.waitForExistence(timeout: screenTimeout), "\(step): no Activity in Settings")
        app.scrollIntoReach(activity)
        activity.tap()
        XCTAssertTrue(
            app.navigationBars["Activity"].waitForExistence(timeout: screenTimeout),
            "\(step): the Activity screen did not show within \(Int(screenTimeout)) s"
        )
    }

    static func row(_ app: XCUIApplication, titled title: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'account-activity-row-' AND label CONTAINS %@", title)
        ).firstMatch
    }

    static func depositReceipt(_ app: XCUIApplication, txn: String, recorder: JourneyRecorder) {
        let row = app.buttons["account-activity-row-\(txn)"]

        recorder.step("S1.1", "the seeded deposit is listed") {
            openActivity(app, step: "S1.1")
            XCTAssertTrue(row.waitForExistence(timeout: screenTimeout), "S1.1: no row for the seeded deposit")
            XCTAssertTrue(row.label.contains("Deposit"), "S1.1: the row does not read Deposit: \(row.label)")
            XCTAssertTrue(row.label.contains("$1.23"), "S1.1: the row does not read $1.23: \(row.label)")
        }

        recorder.step("S1.2", "open the receipt") {
            row.tap()
            let amount = app.element("account-txn-receipt-amount")
            XCTAssertTrue(amount.waitForExistence(timeout: 10), "S1.2: the receipt did not show within 10 s")
            XCTAssertTrue(app.navigationBars["Deposit"].exists, "S1.2: the receipt is not titled Deposit")
            XCTAssertTrue(amount.label.contains("$1.23"), "S1.2: the receipt amount: \(amount.label)")
            XCTAssertTrue(
                app.element("account-txn-receipt-status").label.contains("Done"),
                "S1.2: the status is not Done: \(app.element("account-txn-receipt-status").label)"
            )
            XCTAssertTrue(app.element("account-txn-receipt-time").exists, "S1.2: no time on the receipt")
            let solscan = app.element("account-txn-receipt-solscan")
            XCTAssertTrue(solscan.exists, "S1.2: no View on Solscan")
            XCTAssertTrue(solscan.label.contains("View on Solscan"), "S1.2: the Solscan link: \(solscan.label)")
            JoinJourney.snap(app, "S1-A-receipt")
        }

        recorder.step("S1.3", "close the receipt") {
            app.buttons["account-txn-receipt-done"].tap()
            let gone = NSPredicate(format: "exists == false")
            let closed = XCTNSPredicateExpectation(predicate: gone, object: app.element("account-txn-receipt-amount"))
            XCTAssertEqual(XCTWaiter().wait(for: [closed], timeout: 5), .completed, "S1.3: the receipt did not close")
            XCTAssertTrue(row.waitForExistence(timeout: 5), "S1.3: the deposit row is gone after the receipt closed")
        }
    }

    static func cabalMove(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabal(run: run)
        let funded = row(app, titled: "Funded \(name)")

        recorder.step("S2.1", "the fund is listed") {
            openActivity(app, step: "S2.1")
            XCTAssertTrue(
                funded.waitForExistence(timeout: screenTimeout),
                "S2.1: no row Funded \(name) within \(Int(screenTimeout)) s (known failure, #651)"
            )
        }

        recorder.step("S2.2", "open its receipt") {
            funded.tap()
            let cabalRow = app.element("account-txn-receipt-cabal")
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 10), "S2.2: the receipt has no cabal row")
            XCTAssertTrue(cabalRow.label.contains(name), "S2.2: the cabal row: \(cabalRow.label)")
        }

        recorder.step("S2.3", "open the cabal from the receipt") {
            app.element("account-txn-receipt-cabal").tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: name, timeout: screenTimeout),
                "S2.3: the cabal screen for \(name) did not show within \(Int(screenTimeout)) s"
            )
        }
    }

    static func withdrawal(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "the withdrawal is listed") {
            openActivity(app, step: "S3.1")
            XCTAssertTrue(
                row(app, titled: "Withdrawal").waitForExistence(timeout: screenTimeout),
                "S3.1: no Withdrawal row within \(Int(screenTimeout)) s (known failure, #652)"
            )
        }
    }
}
