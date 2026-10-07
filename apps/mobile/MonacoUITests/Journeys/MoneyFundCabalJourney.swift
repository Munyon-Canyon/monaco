import XCTest

enum MoneyFundCabalJourney {
    static let id = "money/fund-cabal"
    static let version = 2

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA fund \(run)" }

    static func openFund(_ app: XCUIApplication, step: String) {
        let fund = app.buttons["cabal-action-fund"]
        XCTAssertTrue(fund.waitForExistence(timeout: screenTimeout), "\(step): no Add money on the cabal screen")
        fund.tap()
        XCTAssertTrue(
            app.navigationBars["Fund this cabal"].waitForExistence(timeout: screenTimeout),
            "\(step): the Fund this cabal screen did not show within \(Int(screenTimeout)) s"
        )
        XCTAssertTrue(
            app.element("amount-entry-field").waitForExistence(timeout: screenTimeout),
            "\(step): no amount field within \(Int(screenTimeout)) s: is A funded?"
        )
    }

    static func typeAmount(_ app: XCUIApplication, _ amount: String) {
        let field = app.element("amount-entry-field")
        if field.value(forKey: "hasKeyboardFocus") as? Bool != true {
            field.tap()
        }
        if let current = field.value as? String, !current.isEmpty {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
        }
        field.typeText(amount)
    }

    static func pickAmount(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabal(run: run)

        recorder.step("S1.1", "open the cabal") {
            JoinJourney.openBySearch(app, name, step: "S1.1")
        }

        recorder.step("S1.2", "open Fund this cabal") {
            openFund(app, step: "S1.2")
            for preset in ["$25", "$50", "$100", "Max"] {
                XCTAssertTrue(app.buttons[preset].exists, "S1.2: no preset \(preset)")
            }
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("amount-entry-helper"), containing: "available", timeout: screenTimeout),
                "S1.2: the helper does not read … available: \(app.element("amount-entry-helper").label)"
            )
        }

        recorder.step("S1.3", "the notes under the pad") {
            let note =
                "The money leaves your account balance and joins the \(name) pot. Your slice grows by the same amount."
            XCTAssertTrue(app.staticTexts[note].waitForExistence(timeout: 5), "S1.3: no note naming \(name)")
            XCTAssertEqual(
                app.element("fund-cabal-treasury-note").label,
                "To add money to this cabal, use Fund. Sending USDC straight to the treasury will be returned and pauses the cabal's trading.",
                "S1.3: the treasury note"
            )
        }

        recorder.step("S1.4", "more than the balance") {
            app.buttons["$100"].tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("amount-entry-helper"), containing: "Not enough in your account balance.", timeout: 5),
                "S1.4: the helper does not say the balance is too small: \(app.element("amount-entry-helper").label)"
            )
        }

        recorder.step("S1.5", "the button reads the amount") {
            typeAmount(app, "1")
            XCTAssertTrue(
                app.buttons["Add $1 to the pot"].waitForExistence(timeout: 5),
                "S1.5: no button reading Add $1 to the pot within 5 s"
            )
            JoinJourney.snap(app, "S1-A-amount")
        }
    }

    static func addToPot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabal(run: run)

        recorder.step("S2.1", "open the cabal") {
            JoinJourney.openBySearch(app, name, step: "S2.1")
        }

        recorder.step("S2.2", "enter $1") {
            openFund(app, step: "S2.2")
            typeAmount(app, "1")
            let add = app.buttons["Add $1 to the pot"]
            XCTAssertTrue(add.waitForExistence(timeout: screenTimeout), "S2.2: no Add $1 to the pot")
            XCTAssertTrue(add.isEnabled, "S2.2: Add $1 to the pot is disabled (known failure, #608 and #651)")
        }

        recorder.step("S2.3", "add it to the pot") {
            app.buttons["Add $1 to the pot"].tap()
            JoinJourney.waitForToast(app, "Funding this cabal…", step: "S2.3")
            XCTAssertTrue(
                app.staticTexts["Added $1 to \(name)."].firstMatch.waitForExistence(timeout: 120),
                "S2.3: the toast \"Added $1 to \(name).\" did not show within 120 s"
            )
        }
    }
}
