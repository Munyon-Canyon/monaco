import XCTest

enum MoneyCashOutJourney {
    static let id = "money/cash-out"
    static let version = 1

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
            while (balance.label.isEmpty || balance.label.contains("$0.00")) && Date() < deadline {
                RunLoop.current.run(until: Date().addingTimeInterval(0.5))
            }
            XCTAssertFalse(
                balance.label.isEmpty || balance.label.contains("$0.00"),
                "S1.1: the account balance reads \(balance.label): fund A from the Phantom agent wallet first"
            )
        }

        recorder.step("S1.2", "open Cash out") {
            JoinJourney.openBySearch(app, name, step: "S1.2")
            let action = app.element("cabal-action-cash-out")
            XCTAssertTrue(action.waitForExistence(timeout: screenTimeout), "S1.2: no Cash out action")
            action.tap()
            XCTAssertTrue(app.navigationBars["Cash out"].waitForExistence(timeout: 10), "S1.2: no Cash out screen")
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
                app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Your slice is worth $'"))
                    .firstMatch.waitForExistence(timeout: 10),
                "S1.3: no Your slice is worth $… (known failure, #657)"
            )
        }

        recorder.step("S1.4", "pick 25%") {
            app.buttons["25%"].tap()
            let submit = app.buttons["cash-out-submit"]
            XCTAssertTrue(
                JoinJourney.waitForLabel(submit, containing: "Cash out $", timeout: 5),
                "S1.4: the submit reads \(submit.label) (known failure, #657)"
            )
            XCTAssertTrue(submit.isEnabled, "S1.4: the submit is disabled (known failure, #657)")
        }

        recorder.step("S1.5", "cash out") {
            app.buttons["cash-out-submit"].tap()
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Cashing out $'"))
                    .firstMatch.waitForExistence(timeout: 10),
                "S1.5: no Cashing out toast (known failure, #657 and #653)"
            )
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 10), "S1.5: the cabal screen is not back")
        }
    }

    static func withdraw(_ app: XCUIApplication, refundAddress: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open Withdraw from Settings") {
            app.tab("Profile").tap()
            let settings = app.element("profile-settings-row")
            app.scrollIntoReach(settings)
            settings.tap()
            let row = app.element("settings-withdraw")
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S2.1: no Withdraw row in Settings")
            row.tap()
            XCTAssertTrue(app.element("withdraw-address-field").waitForExistence(timeout: 10), "S2.1: no address field")
            XCTAssertTrue(app.buttons["withdraw-continue-button"].exists, "S2.1: no Continue")
        }

        recorder.step("S2.2", "fill in Max and the address") {
            app.buttons["Max"].tap()
            let field = app.element("withdraw-address-field")
            field.tap()
            field.typeText(refundAddress)
            app.buttons["withdraw-continue-button"].tap()
            XCTAssertTrue(
                app.staticTexts["You're withdrawing"].waitForExistence(timeout: 10), "S2.2: no confirm screen")
            JoinJourney.snap(app, "S2-A-withdraw-confirm")
        }

        recorder.step("S2.3", "withdraw") {
            app.buttons["Withdraw"].firstMatch.tap()
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Withdrawing $'"))
                    .firstMatch.waitForExistence(timeout: 10),
                "S2.3: no Withdrawing toast (known failure, #652)"
            )
        }
    }
}
