import XCTest

enum GovernanceProposeSellJourney {
    static let id = "governance/propose-sell"
    static let version = 3

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
            XCTAssertFalse(app.staticTexts["Nothing to sell yet"].exists, "S1.2: the cabal has nothing to sell")
        }

        recorder.step("S1.3", "pick Sell") {
            buy.tapLabel(app, "propose-kind-sell", step: "S1.3")
            buy.expectTitle(app, "Sell", step: "S1.3")
            buy.expectText(app, "What the cabal owns", step: "S1.3")
        }

        recorder.step("S1.4", "pick AAPL") {
            let row = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'propose-sell-AAPL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.4: no AAPL holding within 10 s (#2136)")
            row.tap()
            for chip in ["25%", "50%", "All"] {
                XCTAssertTrue(app.buttons[chip].waitForExistence(timeout: 10), "S1.4: no \(chip) chip")
            }
            let helper = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'The cabal holds'")).firstMatch
            XCTAssertTrue(helper.exists, "S1.4: no The cabal holds helper")
        }

        recorder.step("S1.5", "review 25%") {
            buy.tapLabel(app, "25%", step: "S1.5")
            tapID(app, "propose-amount-review", step: "S1.5")
            buy.expectTitle(app, "Review", step: "S1.5")
            let summary = app.staticTexts.matching(
                NSPredicate(format: "label BEGINSWITH 'Sell ' AND label ENDSWITH ' of AAPL'")
            ).firstMatch
            XCTAssertTrue(summary.waitForExistence(timeout: 10), "S1.5: no Sell … of AAPL")
            buy.expectTexts(app, ["Raises", "Cabal keeps", "Who votes"], step: "S1.5")
        }

        recorder.step("S1.6", "send to the cabal") {
            tapID(app, "propose-review-send", step: "S1.6")
            JoinJourney.waitForToast(app, "Proposal sent to \(name)", step: "S1.6")
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
