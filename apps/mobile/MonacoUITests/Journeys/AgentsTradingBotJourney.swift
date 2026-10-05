import XCTest

enum AgentsTradingBotJourney {
    static let id = "agents/trading-bot"
    static let version = 1

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA bot \(run)" }

    static func openBot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S1.1")
        }

        recorder.step("S1.2", "read the trading bot row") {
            let row = app.element("cabal-agent-row")
            app.scrollIntoReach(row)
            XCTAssertTrue(
                JoinJourney.waitForLabel(row, containing: "$200.00 budget", timeout: 10),
                "S1.2: no trading bot row with a $200.00 budget within 10 s (known failure, #691)")
            XCTAssertTrue(row.label.contains("Active"), "S1.2: the bot row reads '\(row.label)', not Active")
        }

        recorder.step("S1.3", "open the trading bot") {
            app.element("cabal-agent-row").tap()
            let budget = app.element("agent-detail-budget")
            XCTAssertTrue(budget.waitForExistence(timeout: 15), "S1.3: the trading bot screen did not show (#691)")
            XCTAssertTrue(app.navigationBars["Trading bot"].exists, "S1.3: the screen is not titled Trading bot")
            XCTAssertTrue(budget.label.contains("$200.00"), "S1.3: the budget reads '\(budget.label)'")
        }

        recorder.step("S1.4", "show the key") {
            app.element("agent-key-show").tap()
            let hint = "Paste this key into your bot. Anyone in the cabal can copy it here until the bot is removed."
            XCTAssertTrue(app.staticTexts[hint].waitForExistence(timeout: 10), "S1.4: no key hint (#691)")
        }

        recorder.step("S1.5", "copy the connect instructions") {
            let copy = app.element("agent-copy-instructions")
            XCTAssertTrue(copy.label.contains("Copy connect instructions"), "S1.5: the button reads '\(copy.label)'")
            copy.tap()
            JoinJourney.waitForToast(app, "Connect instructions copied", step: "S1.5")
        }
    }

    static func proposeBot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "open the Propose chooser") {
            app.element("cabal-action-propose").tap()
            XCTAssertTrue(
                app.element("propose-chooser").waitForExistence(timeout: 15),
                "S2.2: the Propose chooser did not show within 15 s (known failure, #691 and #613)")
            XCTAssertTrue(app.navigationBars["Propose"].exists, "S2.2: the chooser is not titled Propose")
        }

        recorder.step("S2.3", "tap Add a trading bot") {
            let row = app.element("propose-kind-add-agent")
            XCTAssertTrue(row.waitForExistence(timeout: 5), "S2.3: no Add a trading bot row")
            XCTAssertTrue(row.label.contains("Add a trading bot"), "S2.3: the row reads '\(row.label)'")
            XCTAssertTrue(row.label.contains("Give a bot a budget from the pot"), "S2.3: the row reads '\(row.label)'")
            row.tap()
            XCTAssertTrue(
                app.element("add-agent-name-field").waitForExistence(timeout: 10),
                "S2.3: the add trading bot form did not show within 10 s")
        }

        recorder.step("S2.4", "propose the bot") {
            let name = app.element("add-agent-name-field")
            name.tap()
            name.typeText(cabal(run: run))
            let allocation = app.element("add-agent-allocation-field")
            allocation.tap()
            allocation.typeText("5")
            app.element("add-agent-submit").tap()
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(15) { !app.element("add-agent-submit").exists },
                "S2.4: the add trading bot form did not close within 15 s")
        }
    }
}
