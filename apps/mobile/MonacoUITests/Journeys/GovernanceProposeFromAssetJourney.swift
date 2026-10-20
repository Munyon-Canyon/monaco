import XCTest

enum GovernanceProposeFromAssetJourney {
    static let id = "governance/propose-from-asset"
    static let version = 7

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func assetCabal(run: String) -> String { "QA asset \(run)" }

    static func proposeFromAsset(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = assetCabal(run: run)
        let buy = GovernanceProposeBuyJourney.self

        recorder.step("S1.1", "open GOOGL from the Markets tab") {
            app.tab("Markets").tap()
            let field = app.textFields["monaco-search-field"].firstMatch
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.1: no stock search on the Markets tab")
            field.tap()
            field.typeText("GOOGL")
            let row = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'assets-row-GOOGL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.1: no GOOGL row within 15 s")
            row.tap()
            XCTAssertTrue(app.element("asset-detail-root").waitForExistence(timeout: 15), "S1.1: no asset screen")
            let propose = app.buttons["asset-detail-propose-buy"]
            XCTAssertTrue(propose.waitForExistence(timeout: 15), "S1.1: no Propose a buy")
            app.scrollIntoReach(propose)
            XCTAssertEqual(propose.label, "Propose a buy", "S1.1: the asset CTA")
            buy.expectText(app, "Your cabal votes before anything is bought", step: "S1.1")
        }

        recorder.step("S1.2", "pick a cabal") {
            app.scrollIntoReach(app.buttons["asset-detail-propose-buy"])
            app.buttons["asset-detail-propose-buy"].tap()
            let sheet = app.element("propose-pick-cabal-sheet")
            XCTAssertTrue(sheet.waitForExistence(timeout: 10), "S1.2: no cabal picker sheet within 10 s")
            XCTAssertEqual(sheet.label, "Choose a cabal", "S1.2: the picker sheet title")
            buy.expectText(app, "Which cabal should buy GOOGL?", step: "S1.2")
        }

        recorder.step("S1.3", "pick the run's cabal") {
            let row = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.3: no \(name) in the picker")
            row.tap()
            buy.expectTitle(app, "Buy Alphabet", step: "S1.3")
            for chip in ["$25", "$50", "$100", "Max"] {
                XCTAssertTrue(
                    app.buttons[chip].firstMatch.waitForExistence(timeout: 10), "S1.3: no \(chip) chip on Amount")
            }
            XCTAssertFalse(app.element("monaco-search-field").exists, "S1.3: Amount asks for a stock again")
        }

        recorder.step("S1.4", "an amount over the empty pot is refused") {
            MoneyFundCabalJourney.typeAmount(app, "5")
            buy.expectLabel(app, "Amount exceeds the cash balance.", step: "S1.4")
            XCTAssertFalse(app.buttons["propose-amount-review"].isEnabled, "S1.4: Review is enabled over the pot")
        }
    }
}
