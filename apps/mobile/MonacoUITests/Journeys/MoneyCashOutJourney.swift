import XCTest

enum MoneyCashOutJourney {
    static let id = "money/cash-out"
    static let version = 5

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA cash out \(run)" }

    static func cashOut(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabal(run: run)

        recorder.step("S1.1", "the account balance shows the run's funds") {
            app.tab("Home").tap()
            let balance = app.element("platform-balance-value")
            XCTAssertTrue(balance.waitForExistence(timeout: 60), "S1.1: no account balance on Home within 60 s")
            let deadline = Date().addingTimeInterval(60)
            let reading = { MoneyDepositJourney.balanceReading(balance) }
            while (reading().isEmpty || reading().contains("$0.00")) && Date() < deadline {
                RunLoop.current.run(until: Date().addingTimeInterval(0.5))
            }
            XCTAssertFalse(
                reading().isEmpty || reading().contains("$0.00"),
                "S1.1: the account balance reads \(reading()): fund A from the Phantom agent wallet first"
            )
        }

        recorder.step("S1.2", "open Cash out") {
            JoinJourney.openBySearch(app, name, step: "S1.2")
            let action = app.element("cabal-treasury-cash-out")
            XCTAssertTrue(action.waitForExistence(timeout: screenTimeout), "S1.2: no Cash out action")
            action.tap()
            XCTAssertTrue(
                app.navigationBars["Cash out your share"].waitForExistence(timeout: 10), "S1.2: no Cash out screen")
            XCTAssertTrue(app.element("cash-out-amount").exists, "S1.2: no amount entry")
            for chip in ["25%", "50%", "All"] {
                XCTAssertTrue(app.buttons[chip].exists, "S1.2: no \(chip) chip")
            }
            XCTAssertEqual(
                app.element("cash-out-explainer").label,
                "We sell this much of your slice and move the cash to your account balance. You stay in the cabal.",
                "S1.2: the explainer"
            )
            JoinJourney.snap(app, "S1-A-cash-out")
        }

        recorder.step("S1.3", "the helper reads the slice's worth") {
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label ENDSWITH 'current share value'"))
                    .firstMatch.waitForExistence(timeout: 10),
                "S1.3: no … current share value (known failure, #657)"
            )
        }

        recorder.step("S1.4", "pick 25%") {
            app.buttons["25%"].tap()
            let submit = app.buttons["cash-out-review"]
            XCTAssertTrue(
                JoinJourney.waitForLabel(submit, containing: "Review cash-out", timeout: 5),
                "S1.4: the submit reads \(submit.label) (known failure, #657)"
            )
            XCTAssertTrue(submit.isEnabled, "S1.4: the submit is disabled (known failure, #657)")
        }

        recorder.step("S1.5", "cash out") {
            app.buttons["cash-out-review"].tap()
            app.buttons["cash-out-submit"].tap()
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Cashing out $'"))
                    .firstMatch.waitForExistence(timeout: 10),
                "S1.5: no Cashing out toast (known failure, #657 and #653)"
            )
            XCTAssertTrue(
                app.element("cash-out-progress").waitForExistence(timeout: 10), "S1.5: no cash-out outcome")
        }
    }

    static func withdraw(_ app: XCUIApplication, refundAddress: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "withdraw everything from Settings") {
            MoneyWithdrawJourney.withdrawAll(
                app, to: refundAddress, recorder: MoneyWithdrawJourney.recorder())
        }
    }
}
