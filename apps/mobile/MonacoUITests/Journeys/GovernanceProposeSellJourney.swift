import XCTest

enum GovernanceProposeSellJourney {
    static let id = "governance/propose-sell"
    static let version = 9

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func sellCabal(run: String) -> String { "QA sell \(run)" }

    static func proposeSell(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = sellCabal(run: run)
        let buy = GovernanceProposeBuyJourney.self

        recorder.step("S1.1", "open the cabal as a voter") {
            buy.openCabalAsVoter(app, name, step: "S1.1")
        }

        recorder.step("S1.2", "the chooser offers a sell") {
            buy.openChooser(app, step: "S1.2")
            let sell = app.buttons["propose-kind-sell"].firstMatch
            XCTAssertTrue(sell.waitForExistence(timeout: 10), "S1.2: no propose-kind-sell")
            XCTAssertTrue(sell.isEnabled, "S1.2: the sell row is disabled")
            XCTAssertFalse(app.staticTexts["No holdings to sell"].exists, "S1.2: the cabal has nothing to sell")
        }

        recorder.step("S1.3", "pick Sell") {
            buy.tapLabel(app, "propose-kind-sell", step: "S1.3")
            buy.expectTitle(app, "Sell a holding", step: "S1.3")
        }

        recorder.step("S1.4", "pick AAPL") {
            let row = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'propose-sell-AAPL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.4: no AAPL holding within 10 s (#2136)")
            row.tap()
            XCTAssertTrue(app.element("propose-amount-screen").waitForExistence(timeout: 10), "S1.4: no keypad")
            XCTAssertTrue(app.element("amount-keypad-source").exists, "S1.4: no keypad source")
            for chip in ["25%", "50%", "All"] {
                XCTAssertTrue(
                    app.element("amount-keypad-preset-\(chip)").waitForExistence(timeout: 10), "S1.4: no \(chip) chip")
            }
        }

        recorder.step("S1.5", "review 25%") {
            let amount = app.element("amount-keypad-amount")
            tapID(app, "amount-keypad-preset-25%", step: "S1.5")
            XCTAssertTrue(amount.waitForExistence(timeout: 10), "S1.5: no keypad amount")
            XCTAssertNotEqual(
                amount.value as? String, "$0",
                "S1.5: the 25% chip left the amount at \(String(describing: amount.value))")
            let review = app.buttons["amount-keypad-action"].firstMatch
            let enabled = XCTNSPredicateExpectation(
                predicate: NSPredicate(format: "isEnabled == true"), object: review)
            guard XCTWaiter().wait(for: [enabled], timeout: 15) == .completed else {
                let line = app.element("amount-keypad-line").label
                XCTFail("S1.5: Review proposal never enabled; the line reads \(line)")
                return
            }
            tapID(app, "amount-keypad-action", step: "S1.5")
            buy.expectTitle(sheet: app, step: "S1.5")
            let header = app.element("propose-review-header")
            XCTAssertTrue(
                JoinJourney.waitForLabel(header, containing: "Sell at the market price", timeout: 10),
                "S1.5: the asset row does not read Sell at the market price: \(header.label)")
            buy.expectCabalRow(app, cabal: name, cash: nil, step: "S1.5")
            XCTAssertFalse(
                buy.labelled(app, containing: " cash · ").exists, "S1.5: a sell shows a cash part on the cabal row")
            buy.expectFacts(app, toPass: nil, step: "S1.5")
            XCTAssertFalse(app.element(buy.oldGridID).exists, "S1.5: the old grid is still on the sheet")
        }

        recorder.step("S1.6", "send to the cabal") {
            tapID(app, "propose-review-send", step: "S1.6")
            buy.expectLabel(app, "Proposal sent", step: "S1.6")
        }
    }

    static func tapID(_ app: XCUIApplication, _ id: String, step: String) {
        let target = app.buttons[id].firstMatch
        guard target.waitForExistence(timeout: 10) else {
            XCTFail("\(step): no \(id) to tap within 10 s. On screen:\n\(app.debugDescription.suffix(9000))")
            return
        }
        target.tap()
    }
}
