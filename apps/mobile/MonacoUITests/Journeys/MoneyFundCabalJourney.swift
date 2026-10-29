import XCTest

enum MoneyFundCabalJourney {
    static let id = "money/fund-cabal"
    static let version = 9

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA fund \(run)" }

    static func openFund(_ app: XCUIApplication, step: String) {
        let fund = app.buttons["cabal-action-fund"]
        XCTAssertTrue(fund.waitForExistence(timeout: screenTimeout), "\(step): no Fund on the cabal screen")
        fund.tap()
        XCTAssertTrue(
            app.element("amount-keypad-screen").waitForExistence(timeout: screenTimeout),
            "\(step): the keypad did not show within \(Int(screenTimeout)) s: is A funded?"
        )
    }

    static func typeAmount(_ app: XCUIApplication, _ text: String) {
        for character in text {
            let id = character == "." ? "amount-keypad-dot" : "amount-keypad-\(character)"
            let key = app.element(id)
            XCTAssertTrue(key.waitForExistence(timeout: 10), "no keypad key \(character)")
            key.tap()
        }
    }

    static func clearAmount(_ app: XCUIApplication) {
        let amount = app.element("amount-keypad-amount")
        let delete = app.element("amount-keypad-delete")
        XCTAssertTrue(delete.waitForExistence(timeout: 10), "no amount keypad")
        for _ in 0..<(amount.value as? String ?? "").filter({ $0.isNumber || $0 == "." }).count { delete.tap() }
    }

    static func typeKeypadAmount(_ app: XCUIApplication, _ text: String) {
        clearAmount(app)
        typeAmount(app, text)
    }

    static func confirmSettled(_ app: XCUIApplication, dollars: Int, cabal: String, step: String) {
        let settled = app.element("amount-keypad-settled")
        XCTAssertTrue(
            settled.waitForExistence(timeout: 120), "\(step): the settled button did not show within 120 s")
        XCTAssertTrue(
            settled.label.contains("$\(dollars) to \(cabal)"),
            "\(step): the settled button reads \(settled.label), not \"✓ $\(dollars) to \(cabal)\"")
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("amount-keypad-line"), containing: "Added just now", timeout: 5),
            "\(step): the line does not read Added just now: \(app.element("amount-keypad-line").label)")
    }

    static func confirmDeposit(_ app: XCUIApplication, dollars: Int, cabal: String, step: String) {
        confirmSettled(app, dollars: dollars, cabal: cabal, step: step)
        app.buttons["amount-keypad-close"].tap()
    }

    static func fundOnKeypad(_ app: XCUIApplication, cabalName: String, dollars: String, recorder: JourneyRecorder) {
        openFund(app, step: "fund")
        typeAmount(app, dollars)
        let action = app.buttons["amount-keypad-action"]
        XCTAssertTrue(action.waitForExistence(timeout: screenTimeout), "fund: no Fund $\(dollars) button")
        action.tap()
        confirmDeposit(app, dollars: Int(dollars) ?? 0, cabal: cabalName, step: "fund")
    }

    static func pickAmount(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabal(run: run)

        recorder.step("S1.1", "open the cabal") {
            app.openCabal(named: name)
        }

        recorder.step("S1.2", "Fund opens the keypad") {
            openFund(app, step: "S1.2")
            let source = app.element("amount-keypad-source")
            XCTAssertTrue(source.waitForExistence(timeout: screenTimeout), "S1.2: no source pill")
            XCTAssertTrue(
                JoinJourney.waitForLabel(source, containing: "Cash $", timeout: screenTimeout),
                "S1.2: the source pill does not read Cash $…: \(source.label)")
            XCTAssertTrue(source.label.contains(name), "S1.2: the source pill does not name \(name): \(source.label)")
            XCTAssertEqual(
                app.element("amount-keypad-amount").value as? String, "$0", "S1.2: the amount does not read $0")
            for chip in ["+$1", "+$10", "+$20", "Max"] {
                XCTAssertTrue(app.buttons["amount-keypad-preset-\(chip)"].exists, "S1.2: no chip \(chip)")
            }
            XCTAssertFalse(app.element("amount-keypad-action").exists, "S1.2: a button shows at $0")
        }

        recorder.step("S1.3", "the line shows your share") {
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("amount-keypad-line"), containing: "From your cash · your share", timeout: 5),
                "S1.3: the line does not read From your cash · your share …: \(app.element("amount-keypad-line").label)"
            )
        }

        recorder.step("S1.4", "more than the cash") {
            typeAmount(app, "100")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("amount-keypad-limit-line"), containing: "over", timeout: 5),
                "S1.4: no … available · … over line: \(app.element("amount-keypad-limit-line").label)")
            XCTAssertTrue(
                app.element("amount-keypad-limit-line").label.contains("available"),
                "S1.4: the limit line does not say available")
            XCTAssertFalse(app.buttons["amount-keypad-preset-Max"].exists, "S1.4: the chips stayed over the cash")
            XCTAssertFalse(app.element("amount-keypad-action").exists, "S1.4: the button stayed over the cash")
        }

        recorder.step("S1.5", "the button reads the amount") {
            clearAmount(app)
            typeAmount(app, "1")
            XCTAssertTrue(
                app.buttons["amount-keypad-action"].waitForExistence(timeout: 5),
                "S1.5: no button within 5 s")
            XCTAssertEqual(app.buttons["amount-keypad-action"].label, "Fund $1", "S1.5: the button")
            JoinJourney.snap(app, "S1-A-amount")
        }
    }

    static func addToPot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = cabal(run: run)

        recorder.step("S2.1", "open the cabal") {
            app.openCabal(named: name)
        }

        recorder.step("S2.2", "enter $1") {
            openFund(app, step: "S2.2")
            typeAmount(app, "1")
            let add = app.buttons["amount-keypad-action"]
            XCTAssertTrue(add.waitForExistence(timeout: screenTimeout), "S2.2: no Fund $1")
            XCTAssertEqual(add.label, "Fund $1", "S2.2: the button")
            XCTAssertTrue(add.isEnabled, "S2.2: Fund $1 is disabled")
        }

        recorder.step("S2.3", "fund it") {
            app.buttons["amount-keypad-action"].tap()
            confirmSettled(app, dollars: 1, cabal: name, step: "S2.3")
            app.buttons["amount-keypad-close"].tap()
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: screenTimeout),
                "S2.3: closing did not return to the cabal page")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-pot-value"), containing: "$1.00", timeout: 30),
                "S2.3: the Treasury does not read $1.00 within 30 s: \(app.element("cabal-pot-value").label)")
        }
    }
}
