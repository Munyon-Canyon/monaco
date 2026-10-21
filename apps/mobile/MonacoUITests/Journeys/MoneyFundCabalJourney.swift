import XCTest

enum MoneyFundCabalJourney {
    static let id = "money/fund-cabal"
    static let version = 8

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

    static func confirmDeposit(_ app: XCUIApplication, dollars: Int, cabal: String, step: String) {
        let sheet = app.element("fund-cabal-success-sheet")
        XCTAssertTrue(
            sheet.waitForExistence(timeout: 120), "\(step): the Funding confirmed sheet did not show within 120 s")
        XCTAssertTrue(
            app.staticTexts["Funding confirmed"].firstMatch.exists, "\(step): the sheet is not titled Funding confirmed"
        )
        app.buttons["fund-cabal-done-button"].tap()
        XCTAssertTrue(
            app.staticTexts["Added $\(dollars) to \(cabal)."].firstMatch.waitForExistence(timeout: 120),
            "\(step): the toast \"Added $\(dollars) to \(cabal).\" did not show within 120 s"
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

    static func typeKeypadAmount(_ app: XCUIApplication, _ amount: String) {
        let delete = app.element("amount-keypad-delete")
        XCTAssertTrue(delete.waitForExistence(timeout: 10), "no amount keypad")
        let current = app.element("amount-entry-field").value as? String ?? ""
        for _ in 0..<current.filter({ $0.isNumber || $0 == "." }).count { delete.tap() }
        for character in amount {
            let id = character == "." ? "amount-keypad-dot" : "amount-keypad-\(character)"
            app.element(id).tap()
        }
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

        recorder.step("S1.3", "the yield note under the pad") {
            let note = app.element("fund-cabal-yield-note")
            XCTAssertTrue(note.waitForExistence(timeout: 5), "S1.3: no yield note")
            XCTAssertEqual(
                note.label, "Your cabal earns yield on its USDC. Cash it holds earns 3.5% APY.",
                "S1.3: the yield note")
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
                app.buttons["Fund $1.00"].waitForExistence(timeout: 5),
                "S1.5: no button reading Fund $1.00 within 5 s"
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
            let add = app.buttons["Fund $1.00"]
            XCTAssertTrue(add.waitForExistence(timeout: screenTimeout), "S2.2: no Fund $1.00")
            XCTAssertTrue(add.isEnabled, "S2.2: Fund $1.00 is disabled (known failure, #608 and #651)")
        }

        recorder.step("S2.3", "add it to the pot") {
            app.buttons["Fund $1.00"].tap()
            confirmDeposit(app, dollars: 1, cabal: name, step: "S2.3")
        }
    }
}
